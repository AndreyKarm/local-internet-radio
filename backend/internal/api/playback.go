package api

import (
	"liotom/local-radio/internal/audio"
	"net/http"
	"strconv"
)

func SkipHandler(engine *audio.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if the request method is POST
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		engine.Skip()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"skipped"}`))
	}
}

func PreviousHandler(engine *audio.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 	Check if the request method is POST
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		engine.Previous()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"returned"}`))
	}
}

func LoopHandler(engine *audio.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if the request method is POST
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		engine.ToggleLoop()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"loop_toggled"}`))
	}
}

func PlayByIndexHandler(engine *audio.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if the request method is POST
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

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"playing"}`))
	}
}
