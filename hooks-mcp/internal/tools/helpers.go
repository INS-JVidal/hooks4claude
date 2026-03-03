package tools

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hooks-mcp/internal/dateparse"
	"hooks-mcp/internal/meili"
)

// textResult creates a successful tool result with text content.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

// errResult creates an error tool result. Uses IsError to signal the error
// to the client while keeping the MCP connection alive.
func errResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
		IsError: true,
	}
}

// commonInputs holds the session ID and date range fields shared by multiple tools.
type commonInputs struct {
	SessionID string
	DateRange string
}

// resolvedInputs holds the resolved session ID and parsed date range.
type resolvedInputs struct {
	SessionID string
	DateRange dateparse.DateRange
}

// resolveCommon resolves a session prefix and parses a date range.
// Returns an MCP error result (non-nil) on failure.
func resolveCommon(ctx context.Context, s meili.Searcher, in commonInputs) (resolvedInputs, *mcp.CallToolResult) {
	sessionID := in.SessionID
	if sessionID != "" {
		var err error
		sessionID, err = s.ResolveSessionPrefix(ctx, sessionID)
		if err != nil {
			return resolvedInputs{}, errResult(err.Error())
		}
	}
	dr, err := dateparse.ParseRange(in.DateRange, time.Now())
	if err != nil {
		return resolvedInputs{}, errResult(err.Error())
	}
	return resolvedInputs{SessionID: sessionID, DateRange: dr}, nil
}
