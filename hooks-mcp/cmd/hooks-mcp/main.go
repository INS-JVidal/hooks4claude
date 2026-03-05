package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hooks-mcp/internal/milvus"
	"hooks-mcp/internal/tools"
)

var version = "dev"

func main() {
	// All logging to stderr — stdout is the MCP protocol channel.
	logger := log.New(os.Stderr, "hooks-mcp: ", log.LstdFlags)

	// Configuration from environment.
	milvusURL := envOr("MILVUS_URL", "http://localhost:19530")
	milvusToken := os.Getenv("MILVUS_TOKEN")
	embedSvcURL := os.Getenv("EMBED_SVC_URL")
	eventsCol := envOr("EVENTS_COLLECTION", "hook_events")
	promptsCol := envOr("PROMPTS_COLLECTION", "hook_prompts")
	sessionsCol := envOr("SESSIONS_COLLECTION", "hook_sessions")

	// Health check — fail fast if Milvus is down.
	if err := checkMilvusHealth(milvusURL); err != nil {
		logger.Fatalf("Milvus at %s is not healthy: %v", milvusURL, err)
	}
	logger.Printf("connected to Milvus at %s (v%s)", milvusURL, version)

	// Create embedder only if embed-svc URL is configured.
	var embedder *milvus.Embedder
	if embedSvcURL != "" {
		embedder = milvus.NewEmbedder(embedSvcURL)
		logger.Printf("embedding service configured at %s", embedSvcURL)
	} else {
		logger.Printf("embedding service not configured; semantic search disabled")
	}

	// Create typed Milvus client implementing meili.Searcher.
	searcher := milvus.NewMilvusClient(milvusURL, milvusToken, eventsCol, promptsCol, sessionsCol, embedder)

	// Create MCP server.
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "hooks-mcp",
		Version: version,
	}, nil)

	// Register all tools.
	tools.RegisterAll(server, searcher)

	// Run on stdio transport.
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "hooks-mcp: %v\n", err)
		os.Exit(1)
	}
}

func checkMilvusHealth(baseURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	// Milvus REST v2 collections/list is a POST endpoint.
	resp, err := client.Post(baseURL+"/v2/vectordb/collections/list", "application/json", strings.NewReader("{}"))
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
