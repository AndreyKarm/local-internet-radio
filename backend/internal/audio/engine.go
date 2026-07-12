package audio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"liotom/local-radio/internal/broadcaster"
	"liotom/local-radio/internal/metadata"
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

// NowPlaying describes the track currently on air, plus the full playlist
// order so clients can render a "coming up next" queue.
type NowPlaying struct {
	Key        string
	Title      string
	Artist     string
	Album      string
	Duration   int
	StartedAt  int64
	Queue      []string // track keys in play order
	QueueIndex int      // index of Key within Queue
}

// Engine owns the decode -> encode -> broadcast pipeline and the current
// playback / queue state.
type Engine struct {
	store       *storage.S3Store
	broadcaster *broadcaster.Broadcaster

	trackMu   sync.RWMutex
	current   NowPlaying
	cover     []byte
	coverMIME string

	listMu    sync.Mutex
	listeners []chan NowPlaying
}

func NewEngine(s *storage.S3Store, b *broadcaster.Broadcaster) *Engine {
	return &Engine{store: s, broadcaster: b}
}

// Run drives the whole pipeline until ctx is cancelled: a background
// goroutine feeds raw PCM into an ffmpeg encoder, and the main goroutine
// reads the encoded mp3 stream and fans it out to listeners.
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

// playbackLoop repeatedly refreshes the playlist and plays through it,
// feeding decoded PCM into w in real time.
func (e *Engine) playbackLoop(ctx context.Context, w io.Writer) {
	pace := newPacer(bytesPerSecond)

	for {
		tracks, err := e.refreshPlaylist(ctx)
		if err != nil {
			return // context cancelled
		}
		if len(tracks) == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(playlistRetryDelay):
				continue
			}
		}

		for i, key := range tracks {
			if ctx.Err() != nil {
				return
			}
			e.playTrack(ctx, w, pace, key, i, tracks)
		}
	}
}

// refreshPlaylist lists available tracks (sorted for a stable queue order),
// retrying on failure until ctx is done.
func (e *Engine) refreshPlaylist(ctx context.Context) ([]string, error) {
	for {
		tracks, err := e.store.ListTracks(ctx)
		if err == nil {
			sort.Strings(tracks)
			return tracks, nil
		}
		log.Println("failed to list tracks, retrying in 5s:", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(playlistRetryDelay):
		}
	}
}

// playTrack fetches, decodes and streams a single track's PCM data.
func (e *Engine) playTrack(ctx context.Context, w io.Writer, pace *pacer, key string, index int, queue []string) {
	obj, err := e.store.GetObject(ctx, key)
	if err != nil {
		log.Println("failed to open object:", key, err)
		return
	}
	defer obj.Close()

	track, data, err := metadata.Parse(obj, key)
	if err != nil {
		log.Printf("failed to parse %s: %v\n", key, err)
		return
	}

	duration := probeDuration(ctx, data)
	log.Printf("now playing (%d/%d): %s\n", index+1, len(queue), track.StreamTitle())
	e.setNowPlaying(key, track, duration, queue, index)

	decoded, cleanup, err := decodeMP3(ctx, data)
	if err != nil {
		log.Printf("failed to start decoder for %s: %v\n", key, err)
		return
	}
	defer cleanup()

	if err := pace.copy(ctx, w, decoded); err != nil && ctx.Err() == nil {
		log.Printf("stream error for %s: %v\n", key, err)
	}
}

// decodeMP3 spawns ffmpeg to decode raw mp3 bytes into s16le PCM.
func decodeMP3(ctx context.Context, data []byte) (io.Reader, func(), error) {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "mp3", "-i", "pipe:0",
		"-vn", "-f", "s16le", "-ar", strconv.Itoa(sampleRate), "-ac", strconv.Itoa(channels), "pipe:1",
	)
	cmd.Stdin = bytes.NewReader(data)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	return stdout, func() { _ = cmd.Wait() }, nil
}

// publishEncodedAudio reads encoded mp3 bytes from the encoder and fans
// them out to all connected listeners.
func (e *Engine) publishEncodedAudio(r io.Reader) {
	buf := make([]byte, encodedChunkSize)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			e.broadcaster.Publish(chunk)
		}
		if err != nil {
			return
		}
	}
}

// pacer copies PCM data at a fixed byte rate so playback happens in real
// time regardless of how fast the source can be read/decoded.
type pacer struct {
	bytesPerSecond int
	next           time.Time
}

func newPacer(bytesPerSecond int) *pacer {
	return &pacer{bytesPerSecond: bytesPerSecond, next: time.Now()}
}

func (p *pacer) copy(ctx context.Context, dst io.Writer, src io.Reader) error {
	buf := make([]byte, pcmChunkSize)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, err := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			p.sleepFor(n)
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func (p *pacer) sleepFor(n int) {
	chunkDuration := time.Duration(float64(n) / float64(p.bytesPerSecond) * float64(time.Second))
	p.next = p.next.Add(chunkDuration)
	if sleep := time.Until(p.next); sleep > 0 {
		time.Sleep(sleep)
	} else {
		p.next = time.Now()
	}
}

// --- Now playing / queue state ---

func (e *Engine) setNowPlaying(key string, t *metadata.Track, duration int, queue []string, index int) {
	e.trackMu.Lock()
	e.current = NowPlaying{
		Key:        key,
		Title:      t.Title,
		Artist:     t.Artist,
		Album:      t.Album,
		Duration:   duration,
		StartedAt:  time.Now().UnixMilli(),
		Queue:      queue,
		QueueIndex: index,
	}
	e.cover = t.CoverData
	e.coverMIME = t.CoverMIME
	current := e.current
	e.trackMu.Unlock()

	e.broadcastNowPlaying(current)
}

func (e *Engine) broadcastNowPlaying(np NowPlaying) {
	e.listMu.Lock()
	defer e.listMu.Unlock()
	for _, ch := range e.listeners {
		select {
		case ch <- np:
		default:
		}
	}
}

func (e *Engine) Subscribe() chan NowPlaying {
	ch := make(chan NowPlaying, 1)

	e.listMu.Lock()
	e.listeners = append(e.listeners, ch)
	e.listMu.Unlock()

	// Send the currently playing track immediately upon connecting.
	ch <- e.GetNowPlaying()

	return ch
}

func (e *Engine) Unsubscribe(ch chan NowPlaying) {
	e.listMu.Lock()
	defer e.listMu.Unlock()
	for i, c := range e.listeners {
		if c == ch {
			e.listeners = append(e.listeners[:i], e.listeners[i+1:]...)
			close(ch)
			return
		}
	}
}

func (e *Engine) GetNowPlaying() NowPlaying {
	e.trackMu.RLock()
	defer e.trackMu.RUnlock()
	return e.current
}

// GetQueue returns the full playlist order along with the currently
// playing index.
func (e *Engine) GetQueue() (queue []string, currentIndex int) {
	e.trackMu.RLock()
	defer e.trackMu.RUnlock()
	return e.current.Queue, e.current.QueueIndex
}

func (e *Engine) GetCover() ([]byte, string) {
	e.trackMu.RLock()
	defer e.trackMu.RUnlock()
	return e.cover, e.coverMIME
}

// CurrentStreamTitle implements broadcaster.MetadataProvider.
func (e *Engine) CurrentStreamTitle() string {
	e.trackMu.RLock()
	defer e.trackMu.RUnlock()
	if e.current.Artist != "" && e.current.Artist != "Unknown Artist" {
		return fmt.Sprintf("%s - %s", e.current.Artist, e.current.Title)
	}
	return e.current.Title
}

// probeDuration returns the duration (seconds) of an mp3 blob via ffprobe,
// piping the data directly through stdin to avoid a temp-file round trip.
func probeDuration(ctx context.Context, data []byte) int {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-f", "mp3",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		"pipe:0",
	)
	cmd.Stdin = bytes.NewReader(data)

	out, err := cmd.Output()
	if err != nil {
		log.Printf("ffprobe failed to get duration: %v", err)
		return 0
	}

	outputStr := strings.TrimSpace(string(out))
	if outputStr == "" || outputStr == "N/A" {
		return 0
	}

	duration, err := strconv.ParseFloat(outputStr, 64)
	if err != nil {
		log.Printf("failed to parse ffprobe output %q: %v", outputStr, err)
		return 0
	}
	return int(duration)
}
