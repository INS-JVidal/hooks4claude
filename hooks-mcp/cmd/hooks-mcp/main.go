package main

import (
	"context"
	"fmt"
	"log"
	"os"

	msgo "github.com/meilisearch/meilisearch-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hooks-mcp/internal/meili"
	"hooks-mcp/internal/tools"
)

var version = "dev"

func main() {
	// All logging to stderr — stdout is the MCP protocol channel.
	logger := log.New(os.Stderr, "hooks-mcp: ", log.LstdFlags)

	// Configuration from environment.
	meiliURL := envOr("MEILI_URL", "http://localhost:7700")
	meiliKey := os.Getenv("MEILI_KEY")
	eventsIndex := envOr("MEILI_INDEX", "hook-events")
	promptsIndex := envOr("PROMPTS_INDEX", "hook-prompts")
	sessionsIndex := envOr("SESSIONS_INDEX", "hook-sessions")

	// Health check — fail fast if MeiliSearch is down.
	client := msgo.New(meiliURL, msgo.WithAPIKey(meiliKey))
	if !client.IsHealthy() {
		logger.Fatalf("MeiliSearch at %s is not healthy", meiliURL)
	}
	logger.Printf("connected to MeiliSearch at %s (v%s)", meiliURL, version)

	// Create typed MeiliSearch client.
	searcher := meili.NewMeiliClient(client, eventsIndex, promptsIndex, sessionsIndex)

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

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
