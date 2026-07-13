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

	var wg sync.WaitGroup
	wg.Go(func() {
		defer stdin.Close()
		e.playbackLoop(ctx, stdin)
	})

	e.publishEncodedAudio(stdout)

	_ = encoder.Wait()
	wg.Wait()
}

func (e *Engine) playbackLoop(ctx context.Context, w io.Writer) {
	pace := newPacer(bytesPerSecond)

	if err := e.refreshPlaylist(ctx); err != nil {
		return
	}

	for {
		if err := e.refreshPlaylist(ctx); err != nil {
			return
		}

		e.playlistMu.Lock()
		tracks := make([]TrackInfo, len(e.activePlaylist))
		copy(tracks, e.activePlaylist)
		e.playlistMu.Unlock()

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
		idx := e.current.QueueIndex
		e.trackMu.RUnlock()

		if idx < 0 {
			idx = 0
		}
		if idx >= len(tracks) {
			idx = 0
		}

		currentTrack := tracks[idx]

		trackCtx, cancel := context.WithCancel(ctx)
		e.trackMuControl.Lock()
		e.cancelTrack = cancel
		e.trackMuControl.Unlock()

		err := e.playTrack(trackCtx, w, pace, currentTrack, idx, tracks)

		interrupted := trackCtx.Err() != nil
		cancel()

		if ctx.Err() != nil {
			return
		}

		if !interrupted {
			if err != nil && err != io.EOF {
				log.Printf("stream error: %v\n", err)
			}

			e.trackMu.RLock()
			isLooping := e.loopMode
			e.trackMu.RUnlock()

			if isLooping {
			} else if idx == len(tracks)-1 {
				e.trackMu.Lock()
				e.current.QueueIndex = 0
				if len(tracks) > 0 {
					e.current.Key = tracks[0].Key
				}
				e.trackMu.Unlock()
			} else {
				e.trackMu.Lock()
				e.current.QueueIndex++
				if idx+1 < len(tracks) {
					e.current.Key = tracks[idx+1].Key
				}
				e.trackMu.Unlock()
			}
		}

		if err := e.refreshPlaylist(ctx); err != nil {
			return
		}
	}
}
