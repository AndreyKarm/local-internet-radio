package audio

import (
	"context"
	"fmt"
	"io"
	"liotom/local-radio/internal/metadata"
)

type TrackInfo struct {
	Key      string `json:"key"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	CoverURL string `json:"cover_url"`
}

type NowPlaying struct {
	Key        string
	Title      string
	Artist     string
	Album      string
	Duration   int
	StartedAt  int64
	QueueIndex int
	Looping    bool
	Queue      []TrackInfo
}

func (e *Engine) broadcastNowPlaying(np NowPlaying) {
	e.listMu.Lock()
	defer e.listMu.Unlock()
	// Broadcast the now playing to all listeners
	for _, ch := range e.listeners {
		// Try to send the now playing to the channel
		select {
		// Send the now playing to the channel
		case ch <- np:
			// If the channel is full, drop the message
		default:
		}
	}
}

func (e *Engine) Subscribe() chan NowPlaying {
	// Create a new channel
	ch := make(chan NowPlaying, 1)
	e.listMu.Lock()
	// Append the channel to the listeners list
	e.listeners = append(e.listeners, ch)
	e.listMu.Unlock()
	// Send the current now playing to the channel
	ch <- e.GetNowPlaying()

	e.broadcastNowPlaying(e.GetNowPlaying())

	return ch
}

func (e *Engine) Unsubscribe(ch chan NowPlaying) {
	e.listMu.Lock()
	// Iterate over the listeners
	for i, c := range e.listeners {
		// If the channel matches, remove it and close the channel
		if c == ch {
			// Remove the channel from the listeners list
			e.listeners = append(e.listeners[:i], e.listeners[i+1:]...)
			close(ch)
			e.listMu.Unlock()
			e.broadcastNowPlaying(e.GetNowPlaying())
			return
		}
	}
	e.listMu.Unlock()
}

func (e *Engine) GetNowPlaying() NowPlaying {
	e.trackMu.RLock()
	defer e.trackMu.RUnlock()
	// Return the current now playing
	return e.current
}

func (e *Engine) GetQueue() (queue []TrackInfo, currentIndex int) {
	e.playlistMu.Lock()
	defer e.playlistMu.Unlock()

	e.trackMu.RLock()
	// Get the current index
	idx := e.current.QueueIndex
	e.trackMu.RUnlock()

	// Create a copy of the playlist
	newQueue := make([]TrackInfo, len(e.activePlaylist))
	copy(newQueue, e.activePlaylist)

	// Return the new queue and the current index
	return newQueue, idx
}

func (e *Engine) GetCover() ([]byte, string) {
	e.trackMu.RLock()
	defer e.trackMu.RUnlock()
	// Return the cover and cover MIME type
	return e.cover, e.coverMIME
}

func (e *Engine) CurrentStreamTitle() string {
	e.trackMu.RLock()
	defer e.trackMu.RUnlock()
	// If the artist is not empty and not "Unknown Artist", return the artist and title
	if e.current.Artist != "" && e.current.Artist != "Unknown Artist" {
		return fmt.Sprintf("%s - %s", e.current.Artist, e.current.Title)
	}
	return e.current.Title
}

func (e *Engine) GetNowPlayingCover() ([]byte, string) {
	// Get the cover and cover MIME type
	return e.GetCover()
}

func (e *Engine) GetListenerCount() int {
	return e.broadcaster.ListenerCount()
}

func (e *Engine) GetCoverByKey(ctx context.Context, key string) ([]byte, string, error) {
	// Get the object from the store
	obj, err := e.store.GetObject(ctx, key)
	if err != nil {
		return nil, "", err
	}
	defer obj.Close()

	// Parse the object and get the cover data and cover MIME type
	track, _, err := metadata.Parse(obj, key)
	if err != nil {
		return nil, "", err
	}
	// Return the cover data and cover MIME type
	return track.CoverData, track.CoverMIME, nil
}

func (e *Engine) publishEncodedAudio(r io.Reader) {
	// Create a buffer to hold encoded audio chunks
	buf := make([]byte, encodedChunkSize)
	// Loop until the reader is closed
	for {
		// Read a chunk of encoded audio
		n, err := r.Read(buf)
		// If there is data, broadcast it
		if n > 0 {
			// Create a copy of the data
			chunk := make([]byte, n)
			// Copy the data to the chunk
			copy(chunk, buf[:n])
			// Broadcast the chunk
			e.broadcaster.Publish(chunk)
		}
		if err != nil {
			return
		}
	}
}
