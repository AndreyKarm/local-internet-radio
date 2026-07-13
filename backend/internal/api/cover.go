package api

import (
	"context"
	"net/http"
)

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
