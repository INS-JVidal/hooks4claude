package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hooks-mcp/internal/format"
	"hooks-mcp/internal/meili"
)

type QueryPromptsInput struct {
	ProjectName string `json:"project_name,omitempty" jsonschema:"description=Filter by project name"`
	SessionID   string `json:"session_id,omitempty"   jsonschema:"description=Session ID or prefix (e.g. af7deb64)"`
	DateRange   string `json:"date_range,omitempty"   jsonschema:"description=Date range: today, yesterday, last N days, YYYY-MM-DD, etc."`
	Query       string `json:"query,omitempty"        jsonschema:"description=Full-text search within prompt text"`
	Limit       int    `json:"limit,omitempty"        jsonschema:"description=Max results (default 50, max 200)"`
}

func registerQueryPrompts(server *mcp.Server, searcher meili.Searcher) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "query-prompts",
		Description: "Get user prompts chronologically, optionally grouped by session. Returns prompt text with timestamps.",
	}, makeQueryPrompts(searcher))
}

func makeQueryPrompts(s meili.Searcher) mcp.ToolHandlerFor[QueryPromptsInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in QueryPromptsInput) (*mcp.CallToolResult, any, error) {
		resolved, errRes := resolveCommon(ctx, s, commonInputs{in.SessionID, in.DateRange})
		if errRes != nil {
			return errRes, nil, nil
		}

		hits, total, err := s.SearchPrompts(ctx, meili.PromptSearchOpts{
			Query:       in.Query,
			ProjectName: in.ProjectName,
			SessionID:   resolved.SessionID,
			DateRange:   resolved.DateRange,
			Limit:       int64(in.Limit),
		})
		if err != nil {
			return errResult(err.Error()), nil, nil
		}

		if len(hits) == 0 {
			return textResult("No prompts found."), nil, nil
		}

		// Group prompts by session.
		type sessionGroup struct {
			sessionID string
			prompts   []meili.PromptHit
		}
		groups := make([]sessionGroup, 0)
		groupIdx := make(map[string]int)

		for _, h := range hits {
			idx, ok := groupIdx[h.SessionID]
			if !ok {
				idx = len(groups)
				groupIdx[h.SessionID] = idx
				groups = append(groups, sessionGroup{sessionID: h.SessionID})
			}
			groups[idx].prompts = append(groups[idx].prompts, h)
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Prompts (%d total):\n", total)

		for _, g := range groups {
			fmt.Fprintf(&b, "\n── Session %s ──\n", format.ShortID(g.sessionID))
			for i, p := range g.prompts {
				ts := format.FormatTimestamp(p.TimestampUnix)
				prompt := format.TruncatePrompt(p.Prompt, 200)
				fmt.Fprintf(&b, "  %d. [%s] %s\n", i+1, ts, prompt)
			}
		}

		return textResult(b.String()), nil, nil
	}
}
