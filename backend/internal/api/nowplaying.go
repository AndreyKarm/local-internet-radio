package api

import (
	"encoding/json"
	"liotom/local-radio/internal/audio"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type NowPlayingProvider interface {
	GetNowPlaying() audio.NowPlaying
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
					"queue_index": current.QueueIndex,
					"listeners":   engine.GetListenerCount(),
					"loop":        current.Looping,
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
