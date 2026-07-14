package api

import (
	"context"
	"encoding/json"
	"liotom/local-radio/internal/audio"
	"liotom/local-radio/internal/storage"
	"log"
	"net/http"
)

type PlaylistRefresher interface {
	RefreshPlaylist(ctx context.Context) error
}

func UploadHandler(store *storage.S3Store, refresher PlaylistRefresher) http.HandlerFunc {
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
		log.Println("Song successfuly uploaded")

		if err := refresher.RefreshPlaylist(r.Context()); err != nil {
			log.Printf("failed to refresh playlist after upload: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "success",
			"file":   header.Filename,
		})
	}
}

func DeleteHandler(store *storage.S3Store, engine *audio.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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

		// If the deleted track was playing, skip off of it immediately.
		if engine.GetNowPlaying().Key == key {
			engine.Skip()
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "deleted",
			"key":    key,
		})
	}
}
