package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hooks-mcp/internal/format"
	"hooks-mcp/internal/meili"
)

type RecallContextInput struct {
	Query       string `json:"query"                  jsonschema:"required,description=What you want to recall — e.g. 'when I fixed the auth bug' or 'database migration work'"`
	ProjectName string `json:"project_name,omitempty" jsonschema:"description=Filter by project name"`
	DateRange   string `json:"date_range,omitempty"   jsonschema:"description=Date range"`
	Limit       int    `json:"limit,omitempty"        jsonschema:"description=Max results (default 20, max 50)"`
}

func registerRecallContext(server *mcp.Server, searcher meili.Searcher) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "recall-context",
		Description: "Recall what you were working on when you did something specific. Uses hybrid search (keywords + semantic similarity) via RRF to find the most relevant past events. Returns events with session context.",
	}, makeRecallContext(searcher))
}

func makeRecallContext(s meili.Searcher) mcp.ToolHandlerFor[RecallContextInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in RecallContextInput) (*mcp.CallToolResult, any, error) {
		resolved, errRes := resolveCommon(ctx, s, commonInputs{"", in.DateRange})
		if errRes != nil {
			return errRes, nil, nil
		}

		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		if limit > 50 {
			limit = 50
		}

		opts := meili.EventSearchOpts{
			ProjectName: in.ProjectName,
			DateRange:   resolved.DateRange,
			Limit:       int64(limit),
		}

		hits, err := s.HybridSearch(ctx, in.Query, opts)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		if len(hits) == 0 {
			return textResult("No matching context found. Try a different query or broader date range."), nil, nil
		}

		// Group by session for context display.
		type sessionGroup struct {
			sessionID string
			events    []meili.EventHit
		}
		seen := map[string]int{}
		var groups []sessionGroup
		for _, h := range hits {
			sid := h.SessionID
			if idx, ok := seen[sid]; ok {
				groups[idx].events = append(groups[idx].events, h)
			} else {
				seen[sid] = len(groups)
				groups = append(groups, sessionGroup{sessionID: sid, events: []meili.EventHit{h}})
			}
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Context for %q (%d events across %d sessions):\n\n", in.Query, len(hits), len(groups))

		for _, g := range groups {
			sid := format.ShortID(g.sessionID)
			project := ""
			if len(g.events) > 0 && g.events[0].ProjectName != "" {
				project = " (" + g.events[0].ProjectName + ")"
			}
			fmt.Fprintf(&b, "── Session %s%s ──\n", sid, project)

			for _, h := range g.events {
				ts := format.FormatTimestamp(h.TimestampUnix)
				var detail string
				switch h.HookType {
				case "PreToolUse", "PostToolUse":
					if h.FilePath != "" {
						detail = fmt.Sprintf("%s %s", h.ToolName, h.FilePath)
					} else {
						detail = h.ToolName
					}
				case "PostToolUseFailure":
					detail = fmt.Sprintf("%s: %s", h.ToolName, format.TruncatePrompt(h.ErrorMessage, 80))
				case "UserPromptSubmit":
					detail = format.TruncatePrompt(h.Prompt, 100)
				case "Stop", "SubagentStop":
					detail = fmt.Sprintf("cost=%s tokens=%s/%s", format.FormatCost(h.CostUSD),
						format.FormatTokens(h.InputTokens), format.FormatTokens(h.OutputTokens))
				default:
					detail = h.HookType
				}

				fmt.Fprintf(&b, "  [%s] %-20s  %s\n", ts, h.HookType, detail)
			}
			b.WriteByte('\n')
		}

		return textResult(b.String()), nil, nil
	}
}
