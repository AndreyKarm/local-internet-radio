package api

import (
	"encoding/json"
	"net/http"
)

type FilterProvider interface {
	SetFilter(filter string)
	GetFilter() string
}

func SetFilterHandler(fp FilterProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Expecting JSON: {"filter": "highpass=f=500..."}
		var body struct {
			Filter string `json:"filter"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if body.Filter == "" {
			http.Error(w, "Filter string cannot be empty", http.StatusBadRequest)
			return
		}

		fp.SetFilter(body.Filter)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":     "updating",
			"new_filter": body.Filter,
		})
	}
}

// Also add a GET handler so the frontend knows the current state
func GetFilterHandler(fp FilterProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"filter": fp.GetFilter(),
		})
	}
}
