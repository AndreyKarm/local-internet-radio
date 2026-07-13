package audio

import (
	"context"
	"io"
	"liotom/local-radio/internal/metadata"
	"log"
	"time"
)

func (e *Engine) playTrack(ctx context.Context, w io.Writer, pace *pacer, info TrackInfo, index int, queue []TrackInfo) error {
	obj, err := e.store.GetObject(ctx, info.Key)
	if err != nil {
		return err
	}
	defer obj.Close()

	track, data, err := metadata.Parse(obj, info.Key)
	if err != nil {
		return err
	}

	duration := probeDuration(data)
	log.Printf("now playing (%d/%d): %s\n", index+1, len(queue), track.StreamTitle())

	e.setNowPlayingFromInfo(info.Key, track, duration, queue, index)

	decoded, cleanup, err := decodeMP3(ctx, data)
	if err != nil {
		return err
	}
	defer cleanup()

	return pace.copy(ctx, w, decoded)
}

func (e *Engine) setNowPlayingFromInfo(key string, t *metadata.Track, duration int, queue []TrackInfo, index int) {
	e.trackMu.Lock()
	e.current = NowPlaying{
		Key:        key,
		Title:      t.Title,
		Artist:     t.Artist,
		Album:      t.Album,
		Duration:   duration,
		StartedAt:  time.Now().UnixMilli(),
		QueueIndex: index,
		Looping:    e.loopMode,
	}
	e.cover = t.CoverData
	e.coverMIME = t.CoverMIME
	current := e.current
	e.trackMu.Unlock()

	e.broadcastNowPlaying(current)
}
