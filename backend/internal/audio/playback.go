package audio

import (
	"context"
	"io"
	"liotom/local-radio/internal/metadata"
	"log"
	"time"
)

func (e *Engine) playTrack(ctx context.Context, w io.Writer, pace *pacer, info TrackInfo, index int, queue []TrackInfo) error {
	// Get the object from the store
	obj, err := e.store.GetObject(ctx, info.Key)
	if err != nil {
		return err
	}
	defer obj.Close()

	// Parse the object and get the track data
	track, data, err := metadata.Parse(obj, info.Key)
	if err != nil {
		return err
	}

	// Get the duration of the track
	duration := probeDuration(data)
	log.Printf("now playing (%d/%d): %s\n", index+1, len(queue), track.StreamTitle())

	// Set the now playing
	e.setNowPlayingFromInfo(info.Key, track, duration, index)

	// Decode the audio data
	decoded, cleanup, err := decodeAudioFile(ctx, data)
	if err != nil {
		return err
	}
	// Cleanup the decoded data
	defer cleanup()

	// Copy the decoded data to the writer with the pacer
	return pace.copy(ctx, w, decoded)
}

func (e *Engine) setNowPlayingFromInfo(key string, t *metadata.Track, duration int, index int) {
	e.trackMu.Lock()
	// Set the current now playing
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

	// Update the track
	current := e.current
	e.trackMu.Unlock()

	e.broadcastNowPlaying(current)
}
