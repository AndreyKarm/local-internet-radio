package audio

import (
	"fmt"
	"math/rand/v2"
)

func (e *Engine) Skip() {
	e.playlistMu.Lock()
	e.trackMu.Lock()

	if len(e.activePlaylist) == 0 {
		e.trackMu.Unlock()
		e.playlistMu.Unlock()
		return
	}

	if e.current.QueueIndex < len(e.activePlaylist)-1 {
		e.current.QueueIndex++
	} else {
		e.current.QueueIndex = 0
	}
	e.current.Key = e.activePlaylist[e.current.QueueIndex].Key
	current := e.current

	e.trackMu.Unlock()
	e.playlistMu.Unlock()

	e.broadcastNowPlaying(current)
	go e.interruptCurrentTrack()
}

func (e *Engine) Previous() {
	e.playlistMu.Lock()
	e.trackMu.Lock()

	if len(e.activePlaylist) == 0 {
		e.trackMu.Unlock()
		e.playlistMu.Unlock()
		return
	}

	if e.current.QueueIndex > 0 {
		e.current.QueueIndex--
	} else {
		e.current.QueueIndex = len(e.activePlaylist) - 1
	}
	e.current.Key = e.activePlaylist[e.current.QueueIndex].Key
	current := e.current

	e.trackMu.Unlock()
	e.playlistMu.Unlock()

	e.broadcastNowPlaying(current)
	go e.interruptCurrentTrack()
}

func (e *Engine) ToggleLoop() {
	e.trackMu.Lock()
	e.loopMode = !e.loopMode
	e.current.Looping = e.loopMode
	current := e.current
	e.trackMu.Unlock()
	e.broadcastNowPlaying(current)
}

func (e *Engine) Shuffle() {
	e.playlistMu.Lock()
	defer e.playlistMu.Unlock()

	e.trackMu.Lock()
	if len(e.activePlaylist) <= 1 {
		e.trackMu.Unlock()
		return
	}
	currentIndex := e.current.QueueIndex
	currentKey := e.current.Key
	e.trackMu.Unlock()

	others := make([]TrackInfo, 0, len(e.activePlaylist)-1)
	for _, t := range e.activePlaylist {
		if t.Key != currentKey {
			others = append(others, t)
		}
	}

	rand.Shuffle(len(others), func(i, j int) {
		others[i], others[j] = others[j], others[i]
	})

	newPlaylist := make([]TrackInfo, len(e.activePlaylist))
	othersIdx := 0
	for i := range newPlaylist {
		if i == currentIndex {
			for _, t := range e.activePlaylist {
				if t.Key == currentKey {
					newPlaylist[i] = t
					break
				}
			}
		} else {
			newPlaylist[i] = others[othersIdx]
			othersIdx++
		}
	}

	e.activePlaylist = newPlaylist

	e.trackMu.RLock()
	updatedCurrent := e.current
	e.trackMu.RUnlock()
	e.broadcastNowPlaying(updatedCurrent)
}

func (e *Engine) PlayByIndex(index int) error {
	e.playlistMu.Lock()
	e.trackMu.Lock()

	if index < 0 || index >= len(e.activePlaylist) {
		e.trackMu.Unlock()
		e.playlistMu.Unlock()
		return fmt.Errorf("index %d out of bounds", index)
	}

	e.current.QueueIndex = index
	e.current.Key = e.activePlaylist[index].Key

	e.trackMu.Unlock()
	e.playlistMu.Unlock()

	go e.interruptCurrentTrack()

	return nil
}

func (e *Engine) triggerSkip() {
	select {
	case e.skipSignal <- struct{}{}:
	default:
	}
}

func (e *Engine) interruptCurrentTrack() {
	e.trackMuControl.Lock()
	if e.cancelTrack != nil {
		e.cancelTrack()
	}
	e.trackMuControl.Unlock()
}
