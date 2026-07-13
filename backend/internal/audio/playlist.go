package audio

import (
	"context"
	"fmt"
	"liotom/local-radio/internal/metadata"
	"log"
	"sort"
	"time"
)

func (e *Engine) refreshPlaylist(ctx context.Context) error {
	keys, err := e.store.ListTracks(ctx)
	if err != nil {
		log.Println("failed to list tracks, retrying in 5s:", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(playlistRetryDelay):
			return nil
		}
	}

	sort.Strings(keys)

	keySet := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		keySet[k] = struct{}{}
	}

	e.playlistMu.Lock()
	previous := e.activePlaylist
	e.playlistMu.Unlock()

	var newPlaylist []TrackInfo
	seen := make(map[string]struct{}, len(previous))
	for _, t := range previous {
		if _, ok := keySet[t.Key]; ok {
			newPlaylist = append(newPlaylist, t)
			seen[t.Key] = struct{}{}
		}
	}

	for _, key := range keys {
		if _, ok := seen[key]; ok {
			continue
		}
		info := e.getTrackInfo(ctx, key)
		newPlaylist = append(newPlaylist, info)
	}

	e.playlistMu.Lock()
	e.activePlaylist = newPlaylist
	e.playlistMu.Unlock()

	e.pruneInfoCache(keySet)

	e.syncQueueWithPlaylist(newPlaylist)

	return nil
}

func (e *Engine) pruneInfoCache(keySet map[string]struct{}) {
	e.infoCacheMu.Lock()
	for key := range e.infoCache {
		if _, ok := keySet[key]; !ok {
			delete(e.infoCache, key)
		}
	}
	e.infoCacheMu.Unlock()
}

func (e *Engine) RefreshPlaylist(ctx context.Context) error {
	return e.refreshPlaylist(ctx)
}

func (e *Engine) syncQueueWithPlaylist(tracks []TrackInfo) {
	e.trackMu.Lock()
	defer e.trackMu.Unlock()

	newIndex := 0
	for i, t := range tracks {
		if t.Key == e.current.Key {
			newIndex = i
			break
		}
	}
	e.current.QueueIndex = newIndex

	e.broadcastNowPlaying(e.current)
}

func (e *Engine) getTrackInfo(ctx context.Context, key string) TrackInfo {
	e.infoCacheMu.Lock()
	if info, ok := e.infoCache[key]; ok {
		e.infoCacheMu.Unlock()
		return info
	}
	e.infoCacheMu.Unlock()

	obj, err := e.store.GetObject(ctx, key)
	if err != nil {
		return TrackInfo{Key: key, Title: key}
	}
	defer obj.Close()

	track, _, err := metadata.Parse(obj, key)
	if err != nil {
		return TrackInfo{Key: key, Title: key}
	}

	info := TrackInfo{
		Key:      key,
		Title:    track.Title,
		Artist:   track.Artist,
		Album:    track.Album,
		CoverURL: fmt.Sprintf("/now-playing/cover?key=%s", key),
	}

	e.infoCacheMu.Lock()
	e.infoCache[key] = info
	e.infoCacheMu.Unlock()

	return info
}

func (e *Engine) invalidateTrackInfo(key string) {
	e.infoCacheMu.Lock()
	delete(e.infoCache, key)
	e.infoCacheMu.Unlock()
}
