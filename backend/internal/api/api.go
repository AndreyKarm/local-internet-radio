package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bogem/id3v2/v2"
	"github.com/gorilla/websocket"

	"liotom/local-radio/internal/audio"
	"liotom/local-radio/internal/media"
	"liotom/local-radio/internal/storage"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// ---------- Health ----------

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}

// ---------- Playback control ----------

func SkipHandler(engine *audio.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		engine.Skip()
		respondJSON(w, http.StatusOK, map[string]string{"status": "skipped"})
	}
}

func PreviousHandler(engine *audio.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		engine.Previous()
		respondJSON(w, http.StatusOK, map[string]string{"status": "returned"})
	}
}

func LoopHandler(engine *audio.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		engine.ToggleLoop()
		respondJSON(w, http.StatusOK, map[string]string{"status": "loop_toggled"})
	}
}

func PlayByIndexHandler(engine *audio.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}

		index, err := strconv.Atoi(r.URL.Query().Get("index"))
		if err != nil {
			http.Error(w, "Invalid index parameter", http.StatusBadRequest)
			return
		}

		if err := engine.PlayByIndex(index); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		respondJSON(w, http.StatusOK, map[string]string{"status": "playing"})
	}
}

func ShuffleHandler(engine *audio.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		engine.Shuffle()
		respondJSON(w, http.StatusOK, map[string]string{"status": "shuffled"})
	}
}

type QueueProvider interface {
	GetQueue() (queue []audio.TrackInfo, currentIndex int)
}

func QueueHandler(qp QueueProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		queue, index := qp.GetQueue()
		respondJSON(w, http.StatusOK, map[string]any{
			"queue":         queue,
			"current_index": index,
		})
	}
}

// ---------- Upload ----------

type PlaylistRefresher interface {
	RefreshPlaylist(ctx context.Context) error
}

func UploadHandler(store *storage.S3Store, refresher PlaylistRefresher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}

		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("track")
		if err != nil {
			http.Error(w, "Invalid file", http.StatusBadRequest)
			return
		}

		// Read metadata (incl. cover) from the original upload before
		// ffmpeg's -vn strips the embedded picture.
		track, _, err := media.Parse(file, header.Filename)
		file.Close()
		if err != nil {
			http.Error(w, "Failed to read metadata: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Reopen a fresh reader over the same upload for ffmpeg.
		audioSrc, err := header.Open()
		if err != nil {
			http.Error(w, "Failed to reopen upload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer audioSrc.Close()

		tmpFile, err := os.CreateTemp("", "track-*.mp3")
		if err != nil {
			http.Error(w, "Failed to create temp file: "+err.Error(), http.StatusInternalServerError)
			return
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()
		defer os.Remove(tmpPath)

		if err := media.ToCanonicalMP3(r.Context(), audioSrc, tmpPath); err != nil {
			http.Error(w, "Transcoding failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if err := embedMetadata(tmpPath, track); err != nil {
			log.Printf("failed to embed metadata (continuing without it): %v", err)
		}

		out, err := os.Open(tmpPath)
		if err != nil {
			http.Error(w, "Failed to reopen transcoded file: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer out.Close()

		info, err := out.Stat()
		if err != nil {
			http.Error(w, "Failed to stat transcoded file: "+err.Error(), http.StatusInternalServerError)
			return
		}

		base := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
		key := fmt.Sprintf("%s.mp3", base)

		if err := store.UploadTrack(r.Context(), key, out, info.Size()); err != nil {
			http.Error(w, "Failed to upload to S3: "+err.Error(), http.StatusInternalServerError)
			return
		}

		log.Println("Song successfully uploaded:", key)

		if err := refresher.RefreshPlaylist(r.Context()); err != nil {
			log.Printf("failed to refresh playlist after upload: %v", err)
		}

		respondJSON(w, http.StatusOK, map[string]string{"status": "success", "file": key})
	}
}

// embedMetadata writes title/artist/album and cover art back into the
// ID3v2 tag of the transcoded MP3, restoring the cover that -vn stripped.
func embedMetadata(path string, track *media.Track) error {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: false})
	if err != nil {
		return err
	}
	defer tag.Close()

	tag.SetDefaultEncoding(id3v2.EncodingUTF8)

	if track.Title != "" {
		tag.SetTitle(track.Title)
	}
	if track.Artist != "" && track.Artist != "Unknown Artist" {
		tag.SetArtist(track.Artist)
	}
	if track.Album != "" {
		tag.SetAlbum(track.Album)
	}
	if len(track.CoverData) > 0 {
		mime := track.CoverMIME
		if mime == "" {
			mime = "image/jpeg"
		}
		tag.AddAttachedPicture(id3v2.PictureFrame{
			Encoding:    id3v2.EncodingUTF8,
			MimeType:    mime,
			PictureType: id3v2.PTFrontCover,
			Description: "Front cover",
			Picture:     track.CoverData,
		})
	}

	return tag.Save()
}

func DeleteHandler(store *storage.S3Store, engine *audio.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodDelete) {
			return
		}

		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "Missing key parameter", http.StatusBadRequest)
			return
		}

		if err := store.DeleteTrack(r.Context(), key); err != nil {
			http.Error(w, "Failed to delete track: "+err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("Deleted track: %s", key)

		if err := engine.RefreshPlaylist(r.Context()); err != nil {
			log.Printf("failed to refresh playlist after delete: %v", err)
		}

		if engine.GetNowPlaying().Key == key {
			engine.Skip()
		}

		respondJSON(w, http.StatusOK, map[string]string{"status": "deleted", "key": key})
	}
}

// ---------- Cover ----------

type CoverProvider interface {
	GetNowPlayingCover() ([]byte, string)
	GetCoverByKey(ctx context.Context, key string) ([]byte, string, error)
}

func CoverHandler(cp CoverProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		var data []byte
		var mime string
		var err error

		if key != "" {
			data, mime, err = cp.GetCoverByKey(r.Context(), key)
		} else {
			data, mime = cp.GetNowPlayingCover()
		}

		if err != nil || len(data) == 0 {
			http.NotFound(w, r)
			return
		}
		if mime == "" {
			mime = "image/jpeg"
		}
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(data)
	}
}

// ---------- Now playing ----------

type NowPlayingProvider interface {
	GetNowPlaying() audio.NowPlaying
}

func NowPlayingHandler(np NowPlayingProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		current := np.GetNowPlaying()
		respondJSON(w, http.StatusOK, map[string]any{
			"track":       current.Key,
			"title":       current.Title,
			"artist":      current.Artist,
			"album":       current.Album,
			"cover":       "/now-playing/cover",
			"duration":    current.Duration,
			"started_at":  current.StartedAt,
			"queue_index": current.QueueIndex,
		})
	}
}

func NowPlayingWSHandler(engine *audio.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			return nil
		})

		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				if _, _, err := conn.NextReader(); err != nil {
					return
				}
			}
		}()

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		ch := engine.Subscribe()
		defer engine.Unsubscribe(ch)

		write := func(v any) error {
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			return conn.WriteJSON(v)
		}

		for {
			select {
			case current, ok := <-ch:
				if !ok {
					return
				}
				if err := write(map[string]any{
					"track":       current.Key,
					"title":       current.Title,
					"artist":      current.Artist,
					"album":       current.Album,
					"cover":       "/now-playing/cover",
					"duration":    current.Duration,
					"started_at":  current.StartedAt,
					"queue_index": current.QueueIndex,
					"listeners":   engine.GetListenerCount(),
					"loop":        current.Looping,
					"queue":       current.Queue,
				}); err != nil {
					log.Printf("client disconnected from now-playing ws: %v", err)
					return
				}

			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
				write(map[string]any{"listeners": engine.GetListenerCount()})

			case <-done:
				return
			}
		}
	}
}
