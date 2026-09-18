package audio

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"os"
	"sort"
	"sync"
	"time"

	"liotom/local-radio/internal/broadcaster"
	"liotom/local-radio/internal/media"
	"liotom/local-radio/internal/storage"
)

const playlistRetryDelay = 5 * time.Second

// ---------- Types ----------

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
	QueueIndex int
	Looping    bool
	Queue      []TrackInfo
}

// ---------- Engine ----------

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

	cancelTrack    context.CancelFunc
	trackMuControl sync.Mutex

	infoCache   map[string]TrackInfo
	infoCacheMu sync.Mutex
}

func NewEngine(s *storage.S3Store, b *broadcaster.Broadcaster) *Engine {
	return &Engine{
		store:       s,
		broadcaster: b,
		infoCache:   make(map[string]TrackInfo),
	}
}

func (e *Engine) Run(ctx context.Context) { e.playbackLoop(ctx) }

// ---------- Pacer ----------

type pacer struct {
	next time.Time
}

func newPacer() *pacer { return &pacer{next: time.Now()} }

func (p *pacer) wait(d time.Duration) {
	p.next = p.next.Add(d)
	if sleep := time.Until(p.next); sleep > 0 {
		time.Sleep(sleep)
	} else {
		// Fell behind (e.g. right after a skip/previous) — resync to now.
		p.next = time.Now()
	}
}

// ---------- Playback ----------

func (e *Engine) playbackLoop(ctx context.Context) {
	pace := newPacer()

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
				continue
			}
		}

		e.trackMu.RLock()
		idx := e.current.QueueIndex
		e.trackMu.RUnlock()

		if idx < 0 || idx >= len(tracks) {
			idx = 0
		}

		currentTrack := tracks[idx]

		trackCtx, cancel := context.WithCancel(ctx)
		e.trackMuControl.Lock()
		e.cancelTrack = cancel
		e.trackMuControl.Unlock()

		err := e.playTrack(trackCtx, pace, currentTrack, idx, tracks)

		interrupted := trackCtx.Err() != nil
		cancel()

		if ctx.Err() != nil {
			return
		}

		if !interrupted {
			if err != nil && err != io.EOF {
				log.Printf("stream error: %v\n", err)
			}
			e.advanceAfterTrack(currentTrack, idx, tracks)
		}
	}
}

func (e *Engine) playTrack(ctx context.Context, pace *pacer, info TrackInfo, index int, queue []TrackInfo) error {
	obj, err := e.store.GetObject(ctx, info.Key)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp("", "playing-*.mp3")
	if err != nil {
		obj.Close()
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	defer tmp.Close()

	if _, err := io.Copy(tmp, obj); err != nil {
		obj.Close()
		return err
	}
	obj.Close()

	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}

	br := bufio.NewReader(tmp)

	track, tagLen, err := media.ReadID3v2(br, info.Key)
	if err != nil {
		return err
	}

	// Duration from file size (minus the ID3v2 tag) since it's CBR.
	var duration int
	if st, err := tmp.Stat(); err == nil && st.Size() > int64(tagLen) {
		duration = media.EstimateDuration(st.Size() - int64(tagLen))
	}

	log.Printf("now playing (%d/%d): %s\n", index+1, len(queue), track.StreamTitle())
	e.setNowPlaying(info.Key, track, duration, index)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		frame, data, err := media.ReadFrame(br)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil // clean end of audio
			}
			return err
		}

		e.broadcaster.Publish(data)
		pace.wait(frame.Duration())
	}
}

func (e *Engine) setNowPlaying(key string, t *media.Track, duration, index int) {
	e.trackMu.Lock()
	e.current = NowPlaying{
		Key:        key,
		Title:      t.Title,
		Artist:     t.Artist,
		Album:      t.Album,
		Duration:   duration,
		StartedAt:  time.Now().UnixMilli(),
		QueueIndex: index,
		Looping:    e.loopMode,
	}
	e.cover = t.CoverData
	e.coverMIME = t.CoverMIME

	current := e.current
	e.trackMu.Unlock()

	e.broadcastNowPlaying(current)
}

func (e *Engine) advanceAfterTrack(currentTrack TrackInfo, idx int, tracks []TrackInfo) {
	e.trackMu.Lock()
	defer e.trackMu.Unlock()

	if e.current.Key != currentTrack.Key || e.loopMode {
		return
	}

	if idx == len(tracks)-1 {
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
}

// ---------- Playlist ----------

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

	keySet := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		keySet[k] = struct{}{}
	}

	e.playlistMu.Lock()
	previous := e.activePlaylist
	e.playlistMu.Unlock()

	var newPlaylist []TrackInfo
	seen := make(map[string]struct{}, len(previous))
	for _, t := range previous {
		if _, ok := keySet[t.Key]; ok {
			newPlaylist = append(newPlaylist, t)
			seen[t.Key] = struct{}{}
		}
	}
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			continue
		}
		newPlaylist = append(newPlaylist, e.getTrackInfo(ctx, key))
	}

	e.playlistMu.Lock()
	e.activePlaylist = newPlaylist
	e.playlistMu.Unlock()

	e.pruneInfoCache(keySet)
	e.syncQueueWithPlaylist(newPlaylist)

	return nil
}

func (e *Engine) pruneInfoCache(keySet map[string]struct{}) {
	e.infoCacheMu.Lock()
	for key := range e.infoCache {
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

	newIndex := -1
	for i, t := range tracks {
		if t.Key == e.current.Key {
			newIndex = i
			break
		}
	}
	if newIndex >= 0 {
		e.current.QueueIndex = newIndex
	} else {
		e.current.QueueIndex = 0
	}

	e.current.Queue = tracks
	e.broadcastNowPlaying(e.current)
}

func (e *Engine) getTrackInfo(ctx context.Context, key string) TrackInfo {
	e.infoCacheMu.Lock()
	if info, ok := e.infoCache[key]; ok {
		e.infoCacheMu.Unlock()
		return info
	}
	e.infoCacheMu.Unlock()

	obj, err := e.store.GetObject(ctx, key)
	if err != nil {
		return TrackInfo{Key: key, Title: key}
	}
	defer obj.Close()

	track, _, err := media.ReadID3v2(bufio.NewReader(obj), key)
	if err != nil {
		return TrackInfo{Key: key, Title: key}
	}

	info := TrackInfo{
		Key:      key,
		Title:    track.Title,
		Artist:   track.Artist,
		Album:    track.Album,
		CoverURL: fmt.Sprintf("/now-playing/cover?key=%s", key),
	}

	e.infoCacheMu.Lock()
	e.infoCache[key] = info
	e.infoCacheMu.Unlock()

	return info
}

// ---------- Playback control ----------

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
	e.trackMu.Unlock()

	others := make([]TrackInfo, 0, len(e.activePlaylist)-1)
	for i, track := range e.activePlaylist {
		if i != currentIndex {
			others = append(others, track)
		}
	}
	rand.Shuffle(len(others), func(i, j int) {
		others[i], others[j] = others[j], others[i]
	})

	newPlaylist := make([]TrackInfo, len(e.activePlaylist))
	newPlaylist[0] = e.activePlaylist[currentIndex]
	copy(newPlaylist[1:], others)

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

func (e *Engine) interruptCurrentTrack() {
	e.trackMuControl.Lock()
	if e.cancelTrack != nil {
		e.cancelTrack()
	}
	e.trackMuControl.Unlock()
}

// ---------- Listeners / accessors ----------

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
	e.broadcastNowPlaying(e.GetNowPlaying())
	return ch
}

func (e *Engine) Unsubscribe(ch chan NowPlaying) {
	e.listMu.Lock()
	for i, c := range e.listeners {
		if c == ch {
			e.listeners = append(e.listeners[:i], e.listeners[i+1:]...)
			close(ch)
			e.listMu.Unlock()
			e.broadcastNowPlaying(e.GetNowPlaying())
			return
		}
	}
	e.listMu.Unlock()
}

func (e *Engine) GetNowPlaying() NowPlaying {
	e.trackMu.RLock()
	defer e.trackMu.RUnlock()
	return e.current
}

func (e *Engine) GetQueue() ([]TrackInfo, int) {
	e.playlistMu.Lock()
	defer e.playlistMu.Unlock()

	e.trackMu.RLock()
	idx := e.current.QueueIndex
	e.trackMu.RUnlock()

	queue := make([]TrackInfo, len(e.activePlaylist))
	copy(queue, e.activePlaylist)
	return queue, idx
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

func (e *Engine) NotifyListenerChange() {
	e.broadcastNowPlaying(e.GetNowPlaying())
}

func (e *Engine) GetListenerCount() int {
	return e.broadcaster.ListenerCount()
}

func (e *Engine) GetCoverByKey(ctx context.Context, key string) ([]byte, string, error) {
	obj, err := e.store.GetObject(ctx, key)
	if err != nil {
		return nil, "", err
	}
	defer obj.Close()

	track, _, err := media.ReadID3v2(bufio.NewReader(obj), key)
	if err != nil {
		return nil, "", err
	}
	return track.CoverData, track.CoverMIME, nil
}
