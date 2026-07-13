package audio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
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

// Types
type TrackInfo struct {
	Key      string `json:"key"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	CoverURL string `json:"cover_url"`
}

type NowPlaying struct {
	Key        string
	Title      string
	Artist     string
	Album      string
	Duration   int
	StartedAt  int64
	Queue      []TrackInfo
	QueueIndex int
	Looping    bool
}

type pacer struct {
	bytesPerSecond int
	next           time.Time
}

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
}

// Engine
func NewEngine(s *storage.S3Store, b *broadcaster.Broadcaster) *Engine {
	return &Engine{
		store:       s,
		broadcaster: b,
		skipSignal:  make(chan struct{}, 1),
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

		cancel()

		select {
		case <-ctx.Done():
			return
		case <-e.skipSignal:
			// goto nextIteration
		default:
			if err != nil && err != io.EOF && trackCtx.Err() == nil {
				log.Printf("stream error: %v\n", err)
			}

			e.trackMu.RLock()
			isLooping := e.loopMode
			e.trackMu.RUnlock()

			if isLooping {
				// e.triggerSkip()
			} else if idx == len(tracks)-1 {
				e.trackMu.Lock()
				e.current.QueueIndex = 0
				e.trackMu.Unlock()
				// e.triggerSkip()
			} else {
				e.trackMu.Lock()
				e.current.QueueIndex++
				e.trackMu.Unlock()
				// e.triggerSkip()
			}
		}

		if err := e.refreshPlaylist(ctx); err != nil {
			return
		}
	}
}

func (e *Engine) triggerSkip() {
	select {
	case e.skipSignal <- struct{}{}:
	default:
	}
}

func (e *Engine) Skip() {
	e.trackMu.Lock()
	if e.current.QueueIndex < len(e.current.Queue)-1 {
		e.current.QueueIndex++
	} else {
		e.current.QueueIndex = 0
	}
	e.trackMu.Unlock()
	e.interruptCurrentTrack()
}

func (e *Engine) Previous() {
	e.trackMu.Lock()
	if e.current.QueueIndex > 0 {
		e.current.QueueIndex--
	} else {
		e.current.QueueIndex = len(e.current.Queue) - 1
	}
	e.trackMu.Unlock()
	e.interruptCurrentTrack()
}

func (e *Engine) ToggleLoop() {
	e.trackMu.Lock()
	e.loopMode = !e.loopMode
	e.current.Looping = e.loopMode
	e.trackMu.Unlock()
	e.broadcastNowPlaying(e.GetNowPlaying())
}

func (e *Engine) interruptCurrentTrack() {
	e.trackMuControl.Lock()
	if e.cancelTrack != nil {
		e.cancelTrack()
	}
	e.trackMuControl.Unlock()
	e.triggerSkip()
}

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

	var newPlaylist []TrackInfo
	for _, key := range keys {
		info := e.getTrackInfo(ctx, key)
		newPlaylist = append(newPlaylist, info)
	}

	e.playlistMu.Lock()
	e.activePlaylist = newPlaylist
	e.playlistMu.Unlock()

	e.syncQueueWithPlaylist(newPlaylist)

	return nil
}

func (e *Engine) syncQueueWithPlaylist(tracks []TrackInfo) {
	e.trackMu.Lock()
	currentKey := e.current.Key
	newIndex := 0
	for i, t := range tracks {
		if t.Key == currentKey {
			newIndex = i
			break
		}
	}
	e.current.Queue = tracks
	e.current.QueueIndex = newIndex
	updated := e.current
	e.trackMu.Unlock()

	e.broadcastNowPlaying(updated)
}

func (e *Engine) Shuffle() {
	e.playlistMu.Lock()
	defer e.playlistMu.Unlock()

	if len(e.activePlaylist) <= 1 {
		return
	}

	e.trackMu.RLock()
	currentKey := e.current.Key
	e.trackMu.RUnlock()

	var others []TrackInfo
	var currentTrackInfo *TrackInfo

	for _, t := range e.activePlaylist {
		if t.Key == currentKey {
			temp := t
			currentTrackInfo = &temp
		} else {
			others = append(others, t)
		}
	}

	if currentTrackInfo == nil {
		rand.Shuffle(len(e.activePlaylist), func(i, j int) {
			e.activePlaylist[i], e.activePlaylist[j] = e.activePlaylist[j], e.activePlaylist[i]
		})
	} else {
		rand.Shuffle(len(others), func(i, j int) {
			others[i], others[j] = others[j], others[i]
		})
		newPlaylist := make([]TrackInfo, 0, len(e.activePlaylist))
		newPlaylist = append(newPlaylist, *currentTrackInfo)
		newPlaylist = append(newPlaylist, others...)
		e.activePlaylist = newPlaylist
	}

	e.trackMu.Lock()
	newQueue := make([]TrackInfo, len(e.activePlaylist))
	copy(newQueue, e.activePlaylist)
	e.current.Queue = newQueue
	e.current.QueueIndex = 0
	updatedCurrent := e.current
	e.trackMu.Unlock()

	e.broadcastNowPlaying(updatedCurrent)
}

func (e *Engine) getTrackInfo(ctx context.Context, key string) TrackInfo {
	obj, err := e.store.GetObject(ctx, key)
	if err != nil {
		return TrackInfo{Key: key, Title: key}
	}
	defer obj.Close()

	track, _, err := metadata.Parse(obj, key)
	if err != nil {
		return TrackInfo{Key: key, Title: key}
	}

	return TrackInfo{
		Key:      key,
		Title:    track.Title,
		Artist:   track.Artist,
		Album:    track.Album,
		CoverURL: fmt.Sprintf("/now-playing/cover?key=%s", key),
	}
}

func (e *Engine) playTrack(ctx context.Context, w io.Writer, pace *pacer, info TrackInfo, index int, queue []TrackInfo) error {
	obj, err := e.store.GetObject(ctx, info.Key)
	if err != nil {
		return err
	}
	defer obj.Close()

	track, data, err := metadata.Parse(obj, info.Key)
	if err != nil {
		return err
	}

	duration := probeDuration(ctx, data)
	log.Printf("now playing (%d/%d): %s\n", index+1, len(queue), track.StreamTitle())

	e.setNowPlayingFromInfo(info.Key, track, duration, queue, index)

	decoded, cleanup, err := decodeMP3(ctx, data)
	if err != nil {
		return err
	}
	defer cleanup()

	return pace.copy(ctx, w, decoded)
}

func (e *Engine) setNowPlayingFromInfo(key string, t *metadata.Track, duration int, queue []TrackInfo, index int) {
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
		Looping:    e.loopMode,
	}
	e.cover = t.CoverData
	e.coverMIME = t.CoverMIME
	current := e.current
	e.trackMu.Unlock()

	e.broadcastNowPlaying(current)
}

func (e *Engine) GetCoverByKey(ctx context.Context, key string) ([]byte, string, error) {
	obj, err := e.store.GetObject(ctx, key)
	if err != nil {
		return nil, "", err
	}
	defer obj.Close()

	track, _, err := metadata.Parse(obj, key)
	if err != nil {
		return nil, "", err
	}
	return track.CoverData, track.CoverMIME, nil
}

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

func (e *Engine) setNowPlaying(key string, t *metadata.Track, duration int, queue []TrackInfo, index int) {
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

func (e *Engine) GetQueue() (queue []TrackInfo, currentIndex int) {
	e.trackMu.RLock()
	defer e.trackMu.RUnlock()
	return e.current.Queue, e.current.QueueIndex
}

func (e *Engine) GetCover() ([]byte, string) {
	e.trackMu.RLock()
	defer e.trackMu.RUnlock()
	return e.cover, e.coverMIME
}

func (e *Engine) CurrentStreamTitle() string {
	e.trackMu.RLock()
	defer e.trackMu.RUnlock()
	if e.current.Artist != "" && e.current.Artist != "Unknown Artist" {
		return fmt.Sprintf("%s - %s", e.current.Artist, e.current.Title)
	}
	return e.current.Title
}

func (e *Engine) GetNowPlayingCover() ([]byte, string) {
	return e.GetCover()
}

func (e *Engine) GetListenerCount() int {
	e.listMu.Lock()
	defer e.listMu.Unlock()
	return len(e.listeners)
}

func (e *Engine) RefreshPlaylist(ctx context.Context) error {
	return e.refreshPlaylist(ctx)
}

func (e *Engine) PlayByIndex(index int) error {
	e.trackMu.Lock()
	defer e.trackMu.Unlock()

	if index < 0 || index >= len(e.current.Queue) {
		return fmt.Errorf("index %d out of bounds", index)
	}

	e.current.QueueIndex = index

	go e.interruptCurrentTrack()

	return nil
}

// Pacer
func (p *pacer) copy(ctx context.Context, dst io.Writer, src io.Reader) error {
	buf := make([]byte, pcmChunkSize)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
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

// Helper Functions
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

func newPacer(bytesPerSecond int) *pacer {
	return &pacer{bytesPerSecond: bytesPerSecond, next: time.Now()}
}

func probeDuration(ctx context.Context, data []byte) int {
	tmpfile, err := os.CreateTemp("", "track-*.mp3")
	if err != nil {
		log.Printf("failed to create temp file: %v", err)
		return 0
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write(data); err != nil {
		log.Printf("failed to write to temp file: %v", err)
		tmpfile.Close()
		return 0
	}
	tmpfile.Close()

	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		tmpfile.Name(),
	)

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
