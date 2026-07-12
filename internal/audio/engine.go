package audio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"liotom/local-radio/internal/broadcaster"
	"liotom/local-radio/internal/metadata"
	"liotom/local-radio/internal/storage"
)

type NowPlaying struct {
	Key    string
	Title  string
	Artist string
	Album  string
}

type Engine struct {
	store       *storage.S3Store
	broadcaster *broadcaster.Broadcaster

	current   NowPlaying
	cover     []byte
	coverMIME string
	trackMu   sync.RWMutex
}

func NewEngine(s *storage.S3Store, b *broadcaster.Broadcaster) *Engine {
	return &Engine{store: s, broadcaster: b}
}

func (e *Engine) startDecoder(ctx context.Context, data []byte) (io.ReadCloser, func(), error) {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "warning",
		"-f", "mp3", "-i", "pipe:0",
		"-vn", "-f", "s16le", "-ar", "44100", "-ac", "2", "pipe:1",
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

	cleanup := func() { _ = cmd.Wait() }
	return stdout, cleanup, nil
}

func (e *Engine) Run(ctx context.Context) {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "s16le", "-ar", "44100", "-ac", "2",
		"-i", "pipe:0", "-f", "mp3", "-b:a", "128k", "pipe:1",
	)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatalln("stdin pipe error:", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalln("stdout pipe error:", err)
	}
	if err := cmd.Start(); err != nil {
		log.Fatalln("ffmpeg start error:", err)
	}

	const bytesPerSecond = 176400

	var wg sync.WaitGroup

	wg.Go(func() {
		defer stdin.Close()
		nextChunkTime := time.Now()
		for {
			tracks, err := e.store.ListTracks(ctx)
			if err != nil || len(tracks) == 0 {
				log.Println("no tracks or error, retrying in 5s")
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
					continue
				}
			}

			for _, key := range tracks {
				select {
				case <-ctx.Done():
					return
				default:
				}

				obj, err := e.store.GetObject(ctx, key)
				if err != nil {
					log.Println("Failed to open S3 object:", err)
					continue
				}

				track, data, err := metadata.Parse(obj, key)
				obj.Close()
				if err != nil {
					log.Printf("Failed to read/parse %s: %v\n", key, err)
					continue
				}

				log.Printf("now playing: %s\n", track.StreamTitle())
				e.setNowPlaying(key, track)

				decoderStream, cleanup, err := e.startDecoder(ctx, data)
				if err != nil {
					log.Printf("Failed to start decoder for %s: %v\n", key, err)
					continue
				}

				buf := make([]byte, 4096)
				for {
					n, err := decoderStream.Read(buf)
					if n > 0 {
						if _, writeErr := stdin.Write(buf[:n]); writeErr != nil {
							break
						}
						chunkDuration := time.Duration(float64(n) / float64(bytesPerSecond) * float64(time.Second))
						nextChunkTime = nextChunkTime.Add(chunkDuration)
						sleepTime := time.Until(nextChunkTime)
						if sleepTime > 0 {
							time.Sleep(sleepTime)
						} else {
							nextChunkTime = time.Now()
						}
					}
					if err != nil {
						break
					}
				}
				cleanup()
			}
		}
	})

	buf := make([]byte, 4096)
	for {
		n, err := stdout.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			e.broadcaster.Publish(chunk)
		}
		if err != nil {
			break
		}
	}
	_ = cmd.Wait()
}

func (e *Engine) setNowPlaying(key string, t *metadata.Track) {
	e.trackMu.Lock()
	e.current = NowPlaying{Key: key, Title: t.Title, Artist: t.Artist, Album: t.Album}
	e.cover = t.CoverData
	e.coverMIME = t.CoverMIME
	e.trackMu.Unlock()
}

func (e *Engine) GetNowPlaying() NowPlaying {
	e.trackMu.RLock()
	defer e.trackMu.RUnlock()
	return e.current
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
