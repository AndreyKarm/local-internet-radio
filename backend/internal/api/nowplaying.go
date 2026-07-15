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
		// Upgrade the connection to a WebSocket
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		// Set the read deadline to 60 seconds
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		// Set the pong handler to reset the read deadline
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			return nil
		})

		// Create a channel to signal when the connection is done
		done := make(chan struct{})
		go func() {
			defer close(done)
			// Read messages from the connection
			for {
				// Read the next message
				if _, _, err := conn.NextReader(); err != nil {
					return
				}
			}
		}()

		// Create a ticker to send heartbeat messages
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		// Subscribe to the engine's channel
		ch := engine.Subscribe()
		defer engine.Unsubscribe(ch)

		// Loop until the connection is closed
		for {
			select {
			case current, ok := <-ch: // If a new now playing is available
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
					"queue":       current.Queue,
					"filter":      engine.GetFilter(),
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
