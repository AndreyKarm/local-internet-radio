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
}

func (e *Engine) broadcastNowPlaying(np NowPlaying) {
	e.listMu.Lock()
	defer e.listMu.Unlock()
	for _, ch := range e.listeners {
		select {
		case ch <- np:
		default:
		}
	}
}

func (e *Engine) Subscribe() chan NowPlaying {
	ch := make(chan NowPlaying, 1)
	e.listMu.Lock()
	e.listeners = append(e.listeners, ch)
	e.listMu.Unlock()
	ch <- e.GetNowPlaying()
	return ch
}

func (e *Engine) Unsubscribe(ch chan NowPlaying) {
	e.listMu.Lock()
	defer e.listMu.Unlock()
	for i, c := range e.listeners {
		if c == ch {
			e.listeners = append(e.listeners[:i], e.listeners[i+1:]...)
			close(ch)
			return
		}
	}
}

func (e *Engine) GetNowPlaying() NowPlaying {
	e.trackMu.RLock()
	defer e.trackMu.RUnlock()
	return e.current
}

func (e *Engine) GetQueue() (queue []TrackInfo, currentIndex int) {
	e.playlistMu.Lock()
	defer e.playlistMu.Unlock()

	e.trackMu.RLock()
	idx := e.current.QueueIndex
	e.trackMu.RUnlock()

	newQueue := make([]TrackInfo, len(e.activePlaylist))
	copy(newQueue, e.activePlaylist)

	return newQueue, idx
}

func (e *Engine) GetCover() ([]byte, string) {
	e.trackMu.RLock()
	defer e.trackMu.RUnlock()
	return e.cover, e.coverMIME
}

func (e *Engine) CurrentStreamTitle() string {
	e.trackMu.RLock()
	defer e.trackMu.RUnlock()
	if e.current.Artist != "" && e.current.Artist != "Unknown Artist" {
		return fmt.Sprintf("%s - %s", e.current.Artist, e.current.Title)
	}
	return e.current.Title
}

func (e *Engine) GetNowPlayingCover() ([]byte, string) {
	return e.GetCover()
}

func (e *Engine) GetListenerCount() int {
	e.listMu.Lock()
	defer e.listMu.Unlock()
	return len(e.listeners)
}

func (e *Engine) GetCoverByKey(ctx context.Context, key string) ([]byte, string, error) {
	obj, err := e.store.GetObject(ctx, key)
	if err != nil {
		return nil, "", err
	}
	defer obj.Close()

	track, _, err := metadata.Parse(obj, key)
	if err != nil {
		return nil, "", err
	}
	return track.CoverData, track.CoverMIME, nil
}

func (e *Engine) publishEncodedAudio(r io.Reader) {
	buf := make([]byte, encodedChunkSize)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			e.broadcaster.Publish(chunk)
		}
		if err != nil {
			return
		}
	}
}
