package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"hooks-store/internal/ingest"
	"hooks-store/internal/pubsub"
	"hooks-store/internal/store"
	"hooks-store/internal/tui"

	"hooks4claude/shared/filecache"
)

var version = "dev"

func main() {
	port := flag.String("port", envOrDefault("HOOKS_STORE_PORT", "9800"), "HTTP listen port")
	milvusURL := flag.String("milvus-url", envOrDefault("MILVUS_URL", "http://localhost:19530"), "Milvus endpoint")
	milvusToken := flag.String("milvus-token", envOrDefault("MILVUS_TOKEN", ""), "Milvus API token")
	eventsCol := flag.String("events-col", envOrDefault("MILVUS_EVENTS_COL", "hook_events"), "Milvus events collection name")
	promptsCol := flag.String("prompts-col", envOrDefault("MILVUS_PROMPTS_COL", "hook_prompts"), "Milvus prompts collection name (empty to disable)")
	sessionsCol := flag.String("sessions-col", envOrDefault("MILVUS_SESSIONS_COL", "hook_sessions"), "Milvus sessions collection name (empty to disable)")
	embedURL := flag.String("embed-url", envOrDefault("EMBED_SVC_URL", "http://localhost:8900"), "Embedding service URL (empty to disable)")
	udsSock := flag.String("uds-sock", envOrDefault("HOOKS_STORE_SOCK", ""), "UDS socket path (empty to disable)")
	pubSock := flag.String("pub-sock", envOrDefault("HOOKS_STORE_PUB_SOCK", ""), "UDS pub socket for event subscribers (empty to disable)")
	noCache := flag.Bool("no-cache", false, "Disable file read cache")
	recreateCollections := flag.Bool("recreate-collections", false, "Drop and recreate Milvus collections (needed for schema changes like BM25)")
	headless := flag.Bool("headless", false, "Run without TUI (for scripts and CI)")
	flag.Parse()

	// Connect to Milvus — fail fast if unreachable.
	fmt.Printf("Connecting to Milvus at %s...\n", *milvusURL)
	ms, err := store.NewMilvusStore(*milvusURL, *milvusToken, *eventsCol, *promptsCol, *sessionsCol, *embedURL, *recreateCollections)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer ms.Close()

	// Wrap with buffered store — buffers up to 1000 events when Milvus is down.
	bs := store.NewBufferedStore(ms, 1000, 0)
	defer bs.Close()

	srv := ingest.New(bs)
	srv.SetBufferStats(bs)

	// Pub server for event subscribers (e.g., hooks-monitor).
	var pubServer *pubsub.PubServer
	if *pubSock != "" {
		var err error
		pubServer, err = pubsub.New(*pubSock)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Pub socket error: %v\n", err)
			os.Exit(1)
		}
		defer pubServer.Close()
	}

	// File cache for Read dedup.
	var fc *filecache.SessionFileCache
	if !*noCache {
		fc = filecache.New()
	}

	// Event channel: owned by main, shared between ingest callback and TUI.
	eventCh := make(chan ingest.IngestEvent, 256)
	onIngest := func(evt ingest.IngestEvent, raw []byte) {
		select {
		case eventCh <- evt:
		default: // drop if TUI is slow
		}
		if pubServer != nil {
			go pubServer.Broadcast(raw) // async, never blocks ingest
		}
	}
	srv.SetOnIngest(onIngest)

	httpSrv := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	bindAddr := envOrDefault("BIND_ADDR", "127.0.0.1")
	ln, err := net.Listen("tcp", bindAddr+":"+*port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	actualPort := ln.Addr().(*net.TCPAddr).Port
	listenAddr := fmt.Sprintf("http://localhost:%d", actualPort)

	// Graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	var shutdownOnce sync.Once
	doShutdown := func() {
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		httpSrv.Shutdown(shutdownCtx)
		close(eventCh)
	}

	// Signal handler — SIGINT/SIGTERM triggers shutdown.
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sig)
		select {
		case <-sig:
			shutdownOnce.Do(doShutdown)
		case <-ctx.Done():
		}
	}()

	// Start HTTP server in the background.
	httpErrCh := make(chan error, 1)
	go func() {
		if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			httpErrCh <- err
			shutdownOnce.Do(doShutdown)
		}
	}()

	// Start pub server in background if configured.
	if pubServer != nil {
		go pubServer.Serve(ctx)
		if !*headless {
			fmt.Printf("hooks-store pub socket on %s\n", *pubSock)
		}
	}

	// Optionally start UDS ingest server alongside HTTP.
	if *udsSock != "" {
		udsSrv, err := ingest.NewUDS(*udsSock, bs, fc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "UDS error: %v\n", err)
			os.Exit(1)
		}
		udsSrv.SetOnIngest(onIngest)
		srv.SetUDSStats(udsSrv)
		go func() {
			if err := udsSrv.Serve(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "UDS error: %v\n", err)
			}
		}()
		if !*headless {
			fmt.Printf("hooks-store UDS listening on %s\n", *udsSock)
		}
	}

	if *headless {
		// Headless mode: just log and block until signal.
		fmt.Printf("hooks-store %s listening on %s (headless)\n", version, listenAddr)
		// Drain event channel in background to prevent blocking.
		go func() {
			for range eventCh {
			}
		}()
		<-ctx.Done()
	} else {
		// Run the TUI — blocks until user quits.
		m := tui.NewModel(tui.Config{
			Version:    version,
			MilvusURL:  *milvusURL,
			EventsCol:  *eventsCol,
			ListenAddr: listenAddr,
		}, eventCh, ctx, srv.ErrCount())

		if err := tui.Run(m); err != nil {
			fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		}
	}

	shutdownOnce.Do(doShutdown)

	// Check if HTTP server exited with an error.
	select {
	case err := <-httpErrCh:
		fmt.Fprintf(os.Stderr, "HTTP server error: %v\n", err)
		os.Exit(1)
	default:
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
