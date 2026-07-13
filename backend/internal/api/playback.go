package api

import (
	"liotom/local-radio/internal/audio"
	"log"
	"net/http"
	"strconv"
)

func SkipHandler(engine *audio.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		log.Println("Song Skipped")
		engine.Skip()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"skipped"}`))
	}
}

func PreviousHandler(engine *audio.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		log.Println("Song Previous")
		engine.Previous()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"returned"}`))
	}
}

func LoopHandler(engine *audio.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		log.Println("Song Loop Toggle")
		engine.ToggleLoop()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"loop_toggled"}`))
	}
}

func PlayByIndexHandler(engine *audio.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Get index from query param: /play?index=1
		indexStr := r.URL.Query().Get("index")
		index, err := strconv.Atoi(indexStr)
		if err != nil {
			http.Error(w, "Invalid index parameter", http.StatusBadRequest)
			return
		}

		err = engine.PlayByIndex(index)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		log.Printf("Selected a song at index: %d", index+1)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"playing"}`))
	}
}
