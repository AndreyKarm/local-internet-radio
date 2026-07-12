package api

import (
	"encoding/json"
	"net/http"

	"liotom/local-radio/internal/audio"
)

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}

type NowPlayingProvider interface {
	GetNowPlaying() audio.NowPlaying
}

type CoverProvider interface {
	GetCover() ([]byte, string)
}

func NowPlayingHandler(np NowPlayingProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		current := np.GetNowPlaying()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"track":  current.Key,
			"title":  current.Title,
			"artist": current.Artist,
			"album":  current.Album,
			"cover":  "/now-playing/cover",
		})
	}
}

func CoverHandler(cp CoverProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, mime := cp.GetCover()
		if len(data) == 0 {
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
