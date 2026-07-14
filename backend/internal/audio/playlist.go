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
	// List all the tracks
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

	// Sort the keys
	sort.Strings(keys)

	// Create a set of keys
	keySet := make(map[string]struct{}, len(keys))
	// Add the keys to the set
	for _, k := range keys {
		// Add the key to the set
		keySet[k] = struct{}{}
	}

	e.playlistMu.Lock()
	// Get the previous playlist
	previous := e.activePlaylist
	e.playlistMu.Unlock()

	// Create a new playlist
	var newPlaylist []TrackInfo
	seen := make(map[string]struct{}, len(previous))
	// Add the previous playlist to the new playlist
	for _, t := range previous {
		// If the key is not in the set, skip it
		if _, ok := keySet[t.Key]; ok {
			newPlaylist = append(newPlaylist, t)
			seen[t.Key] = struct{}{}
		}
	}

	// Add the new keys to the new playlist
	for _, key := range keys {
		// If the key is already in the new playlist, skip it
		if _, ok := seen[key]; ok {
			continue
		}
		// Get the track info for the key
		info := e.getTrackInfo(ctx, key)
		newPlaylist = append(newPlaylist, info)
	}

	e.playlistMu.Lock()
	// Update the playlist
	e.activePlaylist = newPlaylist
	e.playlistMu.Unlock()

	// Prune the info cache
	e.pruneInfoCache(keySet)

	// Sync the queue with the playlist
	e.syncQueueWithPlaylist(newPlaylist)

	return nil
}

func (e *Engine) pruneInfoCache(keySet map[string]struct{}) {
	e.infoCacheMu.Lock()
	// Iterate over the info cache
	for key := range e.infoCache {
		// If the key is not in the set, remove it from the cache
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

	// Get the current index
	newIndex := 0
	// Iterate over the tracks
	for i, t := range tracks {
		// If the key matches, set the new index
		if t.Key == e.current.Key {
			newIndex = i
			break
		}
	}
	e.current.QueueIndex = newIndex

	if newIndex != -1 {
		e.current.QueueIndex = newIndex
	} else if e.current.Key == "" && len(tracks) > 0 {
		// nothing has played yet, default sensibly
		e.current.QueueIndex = 0
	}

	e.current.Queue = tracks

	// Set the key to the next song
	e.broadcastNowPlaying(e.current)
}

func (e *Engine) getTrackInfo(ctx context.Context, key string) TrackInfo {
	e.infoCacheMu.Lock()
	// Check if the key is in the cache
	if info, ok := e.infoCache[key]; ok {
		e.infoCacheMu.Unlock()
		return info
	}
	e.infoCacheMu.Unlock()

	// Get the object from the store
	obj, err := e.store.GetObject(ctx, key)
	if err != nil {
		return TrackInfo{Key: key, Title: key}
	}
	defer obj.Close()

	// Parse the object and get the track info
	track, _, err := metadata.Parse(obj, key)
	if err != nil {
		return TrackInfo{Key: key, Title: key}
	}

	// Create a new track info
	info := TrackInfo{
		Key:      key,
		Title:    track.Title,
		Artist:   track.Artist,
		Album:    track.Album,
		CoverURL: fmt.Sprintf("/now-playing/cover?key=%s", key),
	}

	e.infoCacheMu.Lock()
	// Add the track info to the cache
	e.infoCache[key] = info
	e.infoCacheMu.Unlock()

	return info
}

func (e *Engine) invalidateTrackInfo(key string) {
	e.infoCacheMu.Lock()
	// Remove the key from the cache
	delete(e.infoCache, key)
	e.infoCacheMu.Unlock()
}
