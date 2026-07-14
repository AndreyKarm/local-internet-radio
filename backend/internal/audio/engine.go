package audio

import (
	"context"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"liotom/local-radio/internal/broadcaster"
	"liotom/local-radio/internal/storage"
)

const (
	sampleRate         = 44100
	channels           = 2
	bytesPerSample     = 2
	bytesPerSecond     = sampleRate * channels * bytesPerSample // raw PCM byte rate
	pcmChunkSize       = 8192
	encodedChunkSize   = 8192
	playlistRetryDelay = 5 * time.Second
)

// Types

type Engine struct {
	store       *storage.S3Store
	broadcaster *broadcaster.Broadcaster

	trackMu   sync.RWMutex
	current   NowPlaying
	cover     []byte
	coverMIME string

	playlistMu     sync.Mutex
	activePlaylist []TrackInfo
	loopMode       bool

	listMu    sync.Mutex
	listeners []chan NowPlaying

	skipSignal     chan struct{}
	cancelTrack    context.CancelFunc
	trackMuControl sync.Mutex

	infoCache   map[string]TrackInfo
	infoCacheMu sync.Mutex
}

// Engine
func NewEngine(s *storage.S3Store, b *broadcaster.Broadcaster) *Engine {
	return &Engine{
		store:       s,
		broadcaster: b,
		skipSignal:  make(chan struct{}, 1),
		infoCache:   make(map[string]TrackInfo),
	}
}

func (e *Engine) Run(ctx context.Context) {
	// Create a new ffmpeg process
	encoder := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "s16le", "-ar", strconv.Itoa(sampleRate), "-ac", strconv.Itoa(channels),
		"-i", "pipe:0", "-f", "mp3", "-b:a", "128k", "pipe:1",
	)
	encoder.Stderr = os.Stderr

	stdin, err := encoder.StdinPipe()
	if err != nil {
		log.Fatalln("ffmpeg stdin pipe error:", err)
	}
	stdout, err := encoder.StdoutPipe()
	if err != nil {
		log.Fatalln("ffmpeg stdout pipe error:", err)
	}
	if err := encoder.Start(); err != nil {
		log.Fatalln("ffmpeg start error:", err)
	}

	// Start the playback loop
	var wg sync.WaitGroup
	wg.Go(func() {
		defer stdin.Close()
		e.playbackLoop(ctx, stdin)
	})

	// Publish encoded audio to the broadcaster
	e.publishEncodedAudio(stdout)

	// Wait for the process to finish
	_ = encoder.Wait()
	wg.Wait()
}

func (e *Engine) playbackLoop(ctx context.Context, w io.Writer) {
	// Create a new pacer
	pace := newPacer(bytesPerSecond)

	// Refresh the playlist
	if err := e.refreshPlaylist(ctx); err != nil {
		return
	}

	// Loop until the context is canceled
	for {
		// Refresh the playlist
		if err := e.refreshPlaylist(ctx); err != nil {
			return
		}

		e.playlistMu.Lock()
		// Create a copy of the playlist
		tracks := make([]TrackInfo, len(e.activePlaylist))
		copy(tracks, e.activePlaylist)
		e.playlistMu.Unlock()

		// If there are no songs, wait and retry
		if len(tracks) == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(playlistRetryDelay):
				if err := e.refreshPlaylist(ctx); err != nil {
					return
				}
				continue
			}
		}

		e.trackMu.RLock()
		// Get the current index
		idx := e.current.QueueIndex
		e.trackMu.RUnlock()

		// If the index is out of bounds, set it to 0
		if idx < 0 {
			idx = 0
		}
		if idx >= len(tracks) {
			idx = 0
		}

		// Get the current track
		currentTrack := tracks[idx]

		// Create a new context and cancel function
		trackCtx, cancel := context.WithCancel(ctx)
		e.trackMuControl.Lock()
		e.cancelTrack = cancel
		e.trackMuControl.Unlock()

		// Play the track
		err := e.playTrack(trackCtx, w, pace, currentTrack, idx, tracks)

		// Check if the track was interrupted
		interrupted := trackCtx.Err() != nil
		cancel()

		if ctx.Err() != nil {
			return
		}

		// If we are not interrupted, keep playing
		if !interrupted {
			if err != nil && err != io.EOF {
				log.Printf("stream error: %v\n", err)
			}

			e.trackMu.Lock()
			isLooping := e.loopMode
			alreadyChanged := e.current.Key != currentTrack.Key
			defer e.trackMu.Unlock()

			// If we are looping, keep playing
			if alreadyChanged {
				// something else (Skip/Previous/PlayByIndex) already moved us on
			} else if isLooping {
			} else if idx == len(tracks)-1 { // If we are at the end of the playlist
				// Set the current index to 0
				e.current.QueueIndex = 0
				// If there are songs, set the current key to the first one
				if len(tracks) > 0 {
					e.current.Key = tracks[0].Key
				}
			} else { // If we are not at the end of the playlist
				// Increment the current index
				e.current.QueueIndex++
				// If the index is out of bounds, set it to 0
				if idx+1 < len(tracks) {
					e.current.Key = tracks[idx+1].Key
				}
			}
		}

		// Refresh the playlist
		if err := e.refreshPlaylist(ctx); err != nil {
			return
		}
	}
}

func (e *Engine) NotifyListenerChange() {
	e.broadcastNowPlaying(e.GetNowPlaying())
}
