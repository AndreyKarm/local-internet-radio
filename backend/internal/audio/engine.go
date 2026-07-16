package audio

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
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

	// Audio Filters
	filterMu      sync.RWMutex
	currentFilter string
	filterSignal  chan struct{}
}

// Engine
func NewEngine(s *storage.S3Store, b *broadcaster.Broadcaster) *Engine {
	return &Engine{
		store:         s,
		broadcaster:   b,
		skipSignal:    make(chan struct{}, 1),
		infoCache:     make(map[string]TrackInfo),
		filterSignal:  make(chan struct{}, 1),
		currentFilter: "anull", // Clean radio
	}
}

func (e *Engine) Run(ctx context.Context) {
	// Create a new ffmpeg process

	for {
		// Check if context is done before starting a new session
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Create a child context for this specific ffmpeg session
		// This allows us to kill ONLY the ffmpeg process when the filter changes
		sessionCtx, cancelSession := context.WithCancel(ctx)

		watchDone := make(chan struct{})
		go func() {
			defer close(watchDone)
			select {
			case <-sessionCtx.Done():
				return
			case <-e.filterSignal:
				log.Println("Filter change detected. Restarting ffmpeg...")
				cancelSession()
			}
		}()

		// Start the encoder session
		err := e.runEncoderSession(sessionCtx, cancelSession)
		<-watchDone

		if err != nil && err != context.Canceled {
			log.Printf("Encoder session error: %v", err)
		}

		if ctx.Err() != nil {
			return
		}
	}
}

func (e *Engine) runEncoderSession(ctx context.Context, cancelSession context.CancelFunc) error {
	defer cancelSession()

	e.filterMu.RLock()
	filter := e.currentFilter
	e.filterMu.RUnlock()

	log.Printf("Starting ffmpeg with filter: %s", filter)

	var complexFilter string
	if strings.HasPrefix(filter, "[0:a]") || strings.Contains(filter, "[aout]") {
		complexFilter = filter
	} else {
		complexFilter = fmt.Sprintf("[0:a]%s[aout]", filter)
	}

	encoder := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "s16le", "-ar", strconv.Itoa(sampleRate), "-ac", strconv.Itoa(channels),
		"-i", "pipe:0",
		"-filter_complex", complexFilter,
		"-map", "[aout]",
		"-f", "mp3",
		"-b:a", "128k", "pipe:1",
	)
	encoder.Stderr = os.Stderr

	stdin, err := encoder.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := encoder.StdoutPipe()
	if err != nil {
		return err
	}
	if err := encoder.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		defer stdin.Close()
		e.playbackLoop(ctx, stdin)
	})

	e.publishEncodedAudio(stdout)

	_ = encoder.Wait()
	wg.Wait()
	return nil
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

			func() {
				e.trackMu.Lock()
				defer e.trackMu.Unlock()

				isLooping := e.loopMode
				alreadyChanged := e.current.Key != currentTrack.Key

				if alreadyChanged {
					return
				}

				if isLooping {
					return
				} else if idx == len(tracks)-1 {
					e.current.QueueIndex = 0
					if len(tracks) > 0 {
						e.current.Key = tracks[0].Key
					}
				} else {
					e.current.QueueIndex++
					if idx+1 < len(tracks) {
						e.current.Key = tracks[idx+1].Key
					}
				}
			}()
		}

		// Refresh the playlist
		if err := e.refreshPlaylist(ctx); err != nil {
			return
		}
	}
}

func (e *Engine) SetFilter(filterStr string) {
	e.filterMu.Lock()
	log.Println("Locking filter")
	e.currentFilter = filterStr
	e.filterMu.Unlock()
	log.Println("Unlocking filter")

	// Non-blocking send to signal the restart
	select {
	case e.filterSignal <- struct{}{}:
	default:
	}

	e.broadcastNowPlaying(e.GetNowPlaying())
}

func (e *Engine) GetFilter() string {
	e.filterMu.RLock()
	defer e.filterMu.RUnlock()
	return e.currentFilter
}

func (e *Engine) NotifyListenerChange() {
	e.broadcastNowPlaying(e.GetNowPlaying())
}
