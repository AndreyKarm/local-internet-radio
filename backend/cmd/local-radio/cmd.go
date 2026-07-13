package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"liotom/local-radio/internal/api"
	"liotom/local-radio/internal/audio"
	"liotom/local-radio/internal/broadcaster"
	"liotom/local-radio/internal/storage"

	"github.com/rs/cors"
)

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Incoming request: %s %s at %s", r.Method, r.URL.Path, time.Now().Format(time.RFC3339Nano))
		next.ServeHTTP(w, r)
	})
}

func main() {
	store, err := storage.NewS3Store()
	if err != nil {
		log.Fatalf("failed to init storage: %v", err)
	}

	bc := broadcaster.New()
	engine := audio.NewEngine(store, bc)
	bc.SetMetadataProvider(engine)

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	wg.Go(func() {
		engine.Run(ctx)
	})

	mux := http.NewServeMux()
	// REST API
	mux.HandleFunc("/health", api.HealthHandler)
	mux.HandleFunc("/stream", bc.StreamHandler)
	mux.HandleFunc("/play", api.PlayByIndexHandler(engine))
	mux.HandleFunc("/now-playing", api.NowPlayingHandler(engine))
	mux.HandleFunc("/now-playing/cover", api.CoverHandler(engine))
	mux.HandleFunc("/queue", api.QueueHandler(engine))
	mux.HandleFunc("/shuffle", api.ShuffleHandler(engine))
	mux.HandleFunc("/upload", api.UploadHandler(store, engine))
	mux.HandleFunc("/skip", api.SkipHandler(engine))
	mux.HandleFunc("/previous", api.PreviousHandler(engine))
	mux.HandleFunc("/loop", api.LoopHandler(engine))

	// Websockets
	mux.HandleFunc("/ws/now-playing", api.NowPlayingWSHandler(engine))

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "http://127.0.0.1:5173", "http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type", "Range", "Icy-MetaData"},
		ExposedHeaders:   []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           86400,
	})

	handlerWithCors := c.Handler(mux)
	// handlerWithCors := c.Handler(logMiddleware(mux))

	serverPort := int64(8080)
	if p := os.Getenv("PORT"); p != "" {
		if parsed, err := strconv.ParseInt(p, 10, 64); err == nil {
			serverPort = parsed
		} else {
			log.Printf("invalid PORT %q, falling back to %d", p, serverPort)
		}
	}

	server := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", serverPort),
		Handler: handlerWithCors,
	}

	go func() {
		log.Printf("Radio Server listening on 127.0.0.1:%d/stream", serverPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	sig := <-sigChan
	log.Printf("Received signal %v, shutting down gracefully...\n", sig)

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server Shutdown error: %v", err)
	}

	log.Println("Waiting for engine to stop...")
	wg.Wait()

	log.Println("Server stopping...")
}
