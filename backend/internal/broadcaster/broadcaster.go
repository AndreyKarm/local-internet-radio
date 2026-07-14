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
	onChange func()
}

type MetadataProvider interface {
	CurrentStreamTitle() string
}

func New() *Broadcaster {
	// Create a new broadcaster with an empty map of clients
	return &Broadcaster{clients: make(map[chan []byte]struct{})}
}

func (b *Broadcaster) Subscribe() chan []byte {
	// Create a new channel
	ch := make(chan []byte, 8)
	b.mu.Lock()
	// Add the channel to the clients map
	b.clients[ch] = struct{}{}
	b.mu.Unlock()

	b.notifyChange()
	return ch
}

func (b *Broadcaster) Unsubscribe(ch chan []byte) {
	b.mu.Lock()
	// Remove the channel from the clients map
	delete(b.clients, ch)
	b.mu.Unlock()

	b.notifyChange()
}

func (b *Broadcaster) Publish(chunk []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	// Broadcast the chunk to all clients
	for ch := range b.clients {
		// Try to send the chunk to the channel
		select {
		case ch <- chunk:
		default:
		}
	}
}

func (b *Broadcaster) StreamHandler(w http.ResponseWriter, r *http.Request) {
	// Create a new channel
	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	// Check if ICY metadata is requested
	wantsICY := r.Header.Get("Icy-MetaData") == "1"

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	if wantsICY {
		w.Header().Set("icy-metaint", strconv.Itoa(icyMetaInt))
		w.Header().Set("icy-name", "Femboy Radio")
	}

	// Check if the request supports streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Create a writer
	var writer icyInjector
	if wantsICY {
		// Create a new ICY writer
		writer = newICYWriter(w, b.currentTitle)
	} else {
		// Create a plain writer
		writer = plainWriter{w}
	}

	// Write the initial metadata
	for {
		select {
		case chunk := <-ch:
			// Write the chunk to the writer
			if err := writer.Write(chunk); err != nil {
				return
			}
			// Flush the writer
			flusher.Flush()
		case <-r.Context().Done():
			// Unsubscribe from the channel
			return
		}
	}
}

func (p plainWriter) Write(b []byte) error {
	// Write the bytes to the response writer
	_, err := p.w.Write(b)
	return err
}

func (b *Broadcaster) SetMetadataProvider(mp MetadataProvider) {
	b.mu.Lock()
	// Set the metadata provider
	b.metadata = mp
	b.mu.Unlock()
}

func (b *Broadcaster) currentTitle() string {
	b.mu.Lock()
	// Get the metadata provider
	mp := b.metadata
	b.mu.Unlock()
	// If the metadata provider is nil, return an empty string
	if mp == nil {
		return ""
	}
	// Return the current stream title
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
