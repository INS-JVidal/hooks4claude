package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hooks-mcp/internal/meili"
)

// RegisterAll registers all MCP tools on the server.
func RegisterAll(server *mcp.Server, searcher meili.Searcher) {
	registerQuerySessions(server, searcher)
	registerQueryPrompts(server, searcher)
	registerSessionSummary(server, searcher)
	registerProjectActivity(server, searcher)
	registerSearchEvents(server, searcher)
	registerErrorAnalysis(server, searcher)
	registerCostAnalysis(server, searcher)
	registerToolUsage(server, searcher)
}
