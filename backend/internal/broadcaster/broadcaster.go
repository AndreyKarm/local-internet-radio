package broadcaster

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

type Broadcaster struct {
	mu       sync.Mutex
	clients  map[chan []byte]struct{}
	metadata MetadataProvider
	onChange func()
}

type MetadataProvider interface {
	CurrentStreamTitle() string
}

func New() *Broadcaster {
	return &Broadcaster{clients: make(map[chan []byte]struct{})}
}

func (b *Broadcaster) Subscribe() chan []byte {
	ch := make(chan []byte, 8)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	b.notifyChange()
	return ch
}

func (b *Broadcaster) Unsubscribe(ch chan []byte) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
	b.notifyChange()
}

func (b *Broadcaster) Publish(chunk []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		copied := make([]byte, len(chunk))
		copy(copied, chunk)
		select {
		case ch <- copied:
		default:
		}
	}
}

func (b *Broadcaster) StreamHandler(w http.ResponseWriter, r *http.Request) {
	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	wantsICY := r.Header.Get("Icy-MetaData") == "1"

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	if wantsICY {
		w.Header().Set("icy-metaint", strconv.Itoa(icyMetaInt))
		w.Header().Set("icy-name", "Femboy Radio")
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	var writer icyInjector
	if wantsICY {
		writer = newICYWriter(w, b.currentTitle)
	} else {
		writer = plainWriter{w}
	}

	for {
		select {
		case chunk := <-ch:
			if err := writer.Write(chunk); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (b *Broadcaster) SetMetadataProvider(mp MetadataProvider) {
	b.mu.Lock()
	b.metadata = mp
	b.mu.Unlock()
}

func (b *Broadcaster) currentTitle() string {
	b.mu.Lock()
	mp := b.metadata
	b.mu.Unlock()
	if mp == nil {
		return ""
	}
	return mp.CurrentStreamTitle()
}

func (b *Broadcaster) ListenerCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.clients)
}

func (b *Broadcaster) SetOnListenerChange(fn func()) {
	b.mu.Lock()
	b.onChange = fn
	b.mu.Unlock()
}

func (b *Broadcaster) notifyChange() {
	b.mu.Lock()
	fn := b.onChange
	b.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// ---------- ICY metadata injection ----------

const icyMetaInt = 16000

type icyInjector interface {
	Write(p []byte) error
}

type plainWriter struct{ w http.ResponseWriter }

func (p plainWriter) Write(b []byte) error {
	_, err := p.w.Write(b)
	return err
}

type icyWriter struct {
	w           http.ResponseWriter
	getTitle    func() string
	bytesToMeta int
	lastTitle   string
	buf         bytes.Buffer
}

func newICYWriter(w http.ResponseWriter, getTitle func() string) *icyWriter {
	return &icyWriter{w: w, getTitle: getTitle, bytesToMeta: icyMetaInt}
}

func (iw *icyWriter) writeMetaBlock(buf *bytes.Buffer) {
	title := iw.getTitle()
	if title == iw.lastTitle {
		buf.WriteByte(0x00)
		return
	}
	iw.lastTitle = title

	tag := "StreamTitle='" + strings.ReplaceAll(title, "'", "") + "';"
	blockLen := (len(tag) + 15) / 16 // round up to nearest 16 bytes
	buf.WriteByte(byte(blockLen))

	padded := make([]byte, blockLen*16)
	copy(padded, tag)
	buf.Write(padded)
}

func (iw *icyWriter) Write(b []byte) error {
	iw.buf.Reset()
	for len(b) > 0 {
		if iw.bytesToMeta > len(b) {
			iw.buf.Write(b)
			iw.bytesToMeta -= len(b)
			b = nil
			break
		}

		if iw.bytesToMeta > 0 {
			iw.buf.Write(b[:iw.bytesToMeta])
			b = b[iw.bytesToMeta:]
		}

		iw.writeMetaBlock(&iw.buf)
		iw.bytesToMeta = icyMetaInt
	}

	_, err := iw.w.Write(iw.buf.Bytes())
	return err
}
