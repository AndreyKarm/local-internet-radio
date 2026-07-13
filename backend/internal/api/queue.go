package api

import (
	"encoding/json"
	"liotom/local-radio/internal/audio"
	"log"
	"net/http"
)

type QueueProvider interface {
	GetQueue() (queue []audio.TrackInfo, currentIndex int)
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

		log.Println("Shuffle requested...")
		engine.Shuffle()

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"shuffled"}`))
	}
}
