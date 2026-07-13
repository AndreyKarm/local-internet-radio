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

	listMu    sync.Mutex
	listeners []chan NowPlaying
}

// Engine
func NewEngine(s *storage.S3Store, b *broadcaster.Broadcaster) *Engine {
	return &Engine{store: s, broadcaster: b}
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

	for {
		if err := e.refreshPlaylist(ctx); err != nil {
			return
		}

		// 2. Get the tracks to play from our protected activePlaylist
		e.playlistMu.Lock()
		tracks := make([]TrackInfo, len(e.activePlaylist))
		copy(tracks, e.activePlaylist)
		e.playlistMu.Unlock()

		if len(tracks) == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(playlistRetryDelay):
				continue
			}
		}

		for i, info := range tracks {
			if ctx.Err() != nil {
				return
			}
			e.playTrack(ctx, w, pace, info, i, tracks)
		}
	}
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
	return nil
}

func (e *Engine) Shuffle() {
	e.playlistMu.Lock()
	defer e.playlistMu.Unlock()

	if len(e.activePlaylist) <= 1 {
		return
	}

	// 1. Identify the currently playing song key
	e.trackMu.RLock()
	currentKey := e.current.Key
	e.trackMu.RUnlock()

	// 2. Separate the current song from the rest
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
		// 3. Shuffle the "others" slice
		rand.Shuffle(len(others), func(i, j int) {
			others[i], others[j] = others[j], others[i]
		})

		// 4. Rebuild the playlist: [Current Song] + [Shuffled Others]
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

func (e *Engine) playTrack(ctx context.Context, w io.Writer, pace *pacer, info TrackInfo, index int, queue []TrackInfo) {
	obj, err := e.store.GetObject(ctx, info.Key)
	if err != nil {
		log.Println("failed to open object:", info.Key, err)
		return
	}
	defer obj.Close()

	track, data, err := metadata.Parse(obj, info.Key)
	if err != nil {
		log.Printf("failed to parse %s: %v\n", info.Key, err)
		return
	}

	duration := probeDuration(ctx, data)
	log.Printf("now playing (%d/%d): %s\n", index+1, len(queue), track.StreamTitle())

	e.setNowPlayingFromInfo(info.Key, track, duration, queue, index)

	decoded, cleanup, err := decodeMP3(ctx, data)
	if err != nil {
		log.Printf("failed to start decoder for %s: %v\n", info.Key, err)
		return
	}
	defer cleanup()

	if err := pace.copy(ctx, w, decoded); err != nil && ctx.Err() == nil {
		log.Printf("stream error for %s: %v\n", info.Key, err)
	}
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

// Pacer
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
