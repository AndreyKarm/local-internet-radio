package broadcaster

import (
	"net/http"
	"strconv"
	"sync"
)

type Broadcaster struct {
	mu       sync.Mutex
	clients  map[chan []byte]struct{}
	metadata MetadataProvider
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
	return ch
}

func (b *Broadcaster) Unsubscribe(ch chan []byte) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
}

func (b *Broadcaster) Publish(chunk []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- chunk:
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

func (p plainWriter) Write(b []byte) error {
	_, err := p.w.Write(b)
	return err
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
