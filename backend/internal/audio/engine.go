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
	// radioFilter := "highpass=f=400,lowpass=f=3500,volume=1.5"

	// radioFilter := "[0:a]" +
	// 	"highpass=f=500,lowpass=f=2800," +
	// 	"acrusher=level_in=1:level_out=0.7:bits=6:mode=log:aa=1," +
	// 	"vibrato=f=4:d=0.3," +
	// 	"acompressor=threshold=0.1:ratio=9:attack=5:release=50" +
	// 	"[voice];" +
	// 	"anoisesrc=d=9999:amplitude=0.02:c=pink:r=44100,aformat=channel_layouts=stereo" +
	// 	"[noise];" +
	// 	"[voice][noise]amix=inputs=2:duration=first:weights=1 1" +
	// 	"[mixed];" +
	// 	"[mixed]tremolo=f=0.15:d=0.4" +
	// 	"[aout]"

	// radioFilter := "[0:a]" +
	// 	// Brutally narrow telephone-tin-can band
	// 	"highpass=f=800,lowpass=f=2000," +
	// 	// Heavy pre-gain to force clipping/distortion
	// 	"volume=6dB," +
	// 	// Aggressive bitcrush - near-destroyed resolution
	// 	"acrusher=level_in=1.2:level_out=0.5:bits=4:mode=log:aa=0.5," +
	// 	// Hard clipping distortion on top of the crush
	// 	"alimiter=limit=0.6:attack=1:release=20," +
	// 	// Wow & flutter - warped tape/bad tuner drift
	// 	"vibrato=f=6:d=0.6," +
	// 	// Slow deep tremolo - signal fading in/out like bad reception
	// 	"tremolo=f=0.3:d=0.7," +
	// 	// Crush dynamics into oblivion (cheap AM compander, cranked)
	// 	"acompressor=threshold=0.05:ratio=20:attack=2:release=80:makeup=3" +
	// 	"[voice];" +
	// 	// Loud pink noise/hiss bed
	// 	"anoisesrc=d=9999:amplitude=0.05:c=pink:r=44100,aformat=channel_layouts=stereo" +
	// 	"[hiss];" +
	// 	// Occasional white-noise crackle layer for extra grit
	// 	"anoisesrc=d=9999:amplitude=0.03:c=white:r=44100,aformat=channel_layouts=stereo," +
	// 	"highpass=f=3000" +
	// 	"[crackle];" +
	// 	// Mix voice + hiss + crackle together
	// 	"[voice][hiss]amix=inputs=2:duration=first:weights=1 1[mix1];" +
	// 	"[mix1][crackle]amix=inputs=2:duration=first:weights=1 0.6[mixed];" +
	// 	// Random deep signal dropouts - simulates terrible reception
	// 	"[mixed]tremolo=f=0.1:d=0.9" +
	// 	"[aout]"

	// log.Println("FFMPEG FILTER:", radioFilter)

	encoder := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "s16le", "-ar", strconv.Itoa(sampleRate), "-ac", strconv.Itoa(channels),
		"-i", "pipe:0",
		// "-filter_complex", radioFilter,
		"-map", "[aout]",
		"-f", "mp3",
		"-b:a", "128k", "pipe:1",
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

func (e *Engine) NotifyListenerChange() {
	e.broadcastNowPlaying(e.GetNowPlaying())
}
