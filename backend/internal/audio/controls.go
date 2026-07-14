package audio

import (
	"fmt"
	"math/rand/v2"
)

func (e *Engine) Skip() {
	e.playlistMu.Lock()
	e.trackMu.Lock()

	// Check if we have songs
	if len(e.activePlaylist) == 0 {
		e.trackMu.Unlock()
		e.playlistMu.Unlock()
		return
	}

	// If song is not last, skip to next. Else star from beginning
	if e.current.QueueIndex < len(e.activePlaylist)-1 {
		e.current.QueueIndex++
	} else {
		e.current.QueueIndex = 0
	}

	// Set the key to the next song
	e.current.Key = e.activePlaylist[e.current.QueueIndex].Key
	// Update the current track
	current := e.current

	e.trackMu.Unlock()
	e.playlistMu.Unlock()

	// Broadcast the new track
	e.broadcastNowPlaying(current)

	// Interrupt the current track to start the next one
	go e.interruptCurrentTrack()
}

func (e *Engine) Previous() {
	e.playlistMu.Lock()
	e.trackMu.Lock()

	// Check if we have songs
	if len(e.activePlaylist) == 0 {
		e.trackMu.Unlock()
		e.playlistMu.Unlock()
		return
	}

	// If song is not first, select previous. Else select last
	if e.current.QueueIndex > 0 {
		e.current.QueueIndex--
	} else {
		e.current.QueueIndex = len(e.activePlaylist) - 1
	}

	// Set the key to the next song
	e.current.Key = e.activePlaylist[e.current.QueueIndex].Key
	// Update the current track
	current := e.current

	e.trackMu.Unlock()
	e.playlistMu.Unlock()

	// Broadcast the new track
	e.broadcastNowPlaying(current)

	// Interrupt the current track to start the next one
	go e.interruptCurrentTrack()
}

func (e *Engine) ToggleLoop() {
	e.trackMu.Lock()

	// Toggle the loop mode
	e.loopMode = !e.loopMode
	// Update this specific current track to loop mode
	e.current.Looping = e.loopMode
	// Update the current track
	current := e.current
	e.trackMu.Unlock()

	// Broadcast the new track
	e.broadcastNowPlaying(current)
}

func (e *Engine) Shuffle() {
	e.playlistMu.Lock()
	defer e.playlistMu.Unlock()

	e.trackMu.Lock()
	// Check if we have songs, and more than one
	if len(e.activePlaylist) <= 1 {
		e.trackMu.Unlock()
		return
	}

	// Get the current index and key
	currentIndex := e.current.QueueIndex
	e.trackMu.Unlock()

	// Create a new playlist with all the songs except the current one
	others := make([]TrackInfo, 0, len(e.activePlaylist)-1)
	for i, track := range e.activePlaylist {
		if i != currentIndex {
			others = append(others, track)
		}
	}

	// Shuffle the playlist
	rand.Shuffle(len(others), func(i, j int) {
		others[i], others[j] = others[j], others[i]
	})

	// Create a new playlist with the shuffled playlist and the current song
	newPlaylist := make([]TrackInfo, len(e.activePlaylist))
	// Apply the current song to the first position
	newPlaylist[0] = e.activePlaylist[currentIndex]
	othersIdx := 0
	for i := 1; i < len(newPlaylist); i++ {
		newPlaylist[i] = others[othersIdx]
		othersIdx++
	}

	// Update the playlist
	e.activePlaylist = newPlaylist

	e.trackMu.RLock()
	// Update the current track
	updatedCurrent := e.current
	e.trackMu.RUnlock()

	// Broadcast the new track
	e.broadcastNowPlaying(updatedCurrent)
}

func (e *Engine) PlayByIndex(index int) error {
	e.playlistMu.Lock()
	e.trackMu.Lock()

	// Check if index is bigger then the playlist size or lower then 0
	if index < 0 || index >= len(e.activePlaylist) {
		e.trackMu.Unlock()
		e.playlistMu.Unlock()
		return fmt.Errorf("index %d out of bounds", index)
	}

	// Set it as a new queue index
	e.current.QueueIndex = index
	// Set the current track key to the one we found in the playlist by given index
	e.current.Key = e.activePlaylist[index].Key

	e.trackMu.Unlock()
	e.playlistMu.Unlock()

	// Interrupt the current song and start playing the newely selected one
	go e.interruptCurrentTrack()

	return nil
}

func (e *Engine) interruptCurrentTrack() {
	e.trackMuControl.Lock()
	// Check if we aren't already skipping
	if e.cancelTrack != nil {
		e.cancelTrack()
	}
	e.trackMuControl.Unlock()
}
