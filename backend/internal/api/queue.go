package api

import (
	"encoding/json"
	"liotom/local-radio/internal/audio"
	"net/http"
)

type QueueProvider interface {
	GetQueue() (queue []audio.TrackInfo, currentIndex int)
}

func QueueHandler(qp QueueProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if the request method is GET
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Set the content type to JSON
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
		// Check if the request method is POST
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		engine.Shuffle()

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"shuffled"}`))
	}
}
