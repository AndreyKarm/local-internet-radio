package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"liotom/local-radio/internal/audio"
	"liotom/local-radio/internal/storage"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Types
type NowPlayingProvider interface {
	GetNowPlaying() audio.NowPlaying
}

type QueueProvider interface {
	GetQueue() (queue []audio.TrackInfo, currentIndex int)
}

type CoverProvider interface {
	GetNowPlayingCover() ([]byte, string)
	GetCoverByKey(ctx context.Context, key string) ([]byte, string, error)
}

// REST API
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}

func NowPlayingHandler(np NowPlayingProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		current := np.GetNowPlaying()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"track":       current.Key,
			"title":       current.Title,
			"artist":      current.Artist,
			"album":       current.Album,
			"cover":       "/now-playing/cover",
			"duration":    current.Duration,
			"started_at":  current.StartedAt,
			"queue":       current.Queue,
			"queue_index": current.QueueIndex,
		})
	}
}

func CoverHandler(cp CoverProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		var data []byte
		var mime string
		var err error

		if key != "" {
			// Fetch specific cover for a queue item
			data, mime, err = cp.GetCoverByKey(r.Context(), key)
		} else {
			// Fetch the currently playing cover
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

func QueueHandler(qp QueueProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		queue, index := qp.GetQueue()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"queue":         queue,
			"current_index": index,
		})
	}
}

func ShuffleHandler(engine *audio.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		engine.Shuffle()

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"shuffled"}`))
	}
}

func UploadHandler(store *storage.S3Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Restrict to POST methods
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse the multipart form with a 32 MB max memory limit
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Retrieve the file from form data
		file, header, err := r.FormFile("track")
		if err != nil {
			http.Error(w, "Invalid file", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Stream the file directly to MinIO
		err = store.UploadTrack(r.Context(), header.Filename, file, header.Size)
		if err != nil {
			http.Error(w, "Failed to upload to S3", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "success",
			"file":   header.Filename,
		})
	}
}

// Websockets
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

		for {
			select {
			case current, ok := <-ch:
				if !ok {
					return
				}
				conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				err := conn.WriteJSON(map[string]any{
					"track":       current.Key,
					"title":       current.Title,
					"artist":      current.Artist,
					"album":       current.Album,
					"cover":       "/now-playing/cover",
					"duration":    current.Duration,
					"started_at":  current.StartedAt,
					"queue":       current.Queue,
					"queue_index": current.QueueIndex,
					"listeners":   engine.GetListenerCount(),
				})
				if err != nil {
					log.Printf("client disconnected from now-playing ws: %v", err)
					return
				}
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
				conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				conn.WriteJSON(map[string]any{
					"listeners": engine.GetListenerCount(),
				})
			case <-done:
				return
			}
		}
	}
}
