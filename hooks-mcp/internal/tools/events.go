package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hooks-mcp/internal/format"
	"hooks-mcp/internal/meili"
)

type SearchEventsInput struct {
	Query       string `json:"query"                  jsonschema:"required,description=Search query (searches data_flat, prompt, error_message, etc.)"`
	ProjectName string `json:"project_name,omitempty" jsonschema:"description=Filter by project name"`
	SessionID   string `json:"session_id,omitempty"   jsonschema:"description=Session ID or prefix"`
	HookType    string `json:"hook_type,omitempty"    jsonschema:"description=Filter by hook type: PreToolUse, PostToolUse, Stop, UserPromptSubmit, etc."`
	ToolName    string `json:"tool_name,omitempty"    jsonschema:"description=Filter by tool name: Read, Edit, Bash, Grep, Glob, Write, Agent, etc."`
	DateRange   string `json:"date_range,omitempty"   jsonschema:"description=Date range"`
	Limit       int    `json:"limit,omitempty"        jsonschema:"description=Max results (default 20, max 100)"`
}

func registerSearchEvents(server *mcp.Server, searcher meili.Searcher) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search-events",
		Description: "Full-text search across all hook event data. Searches data_flat (all nested values), prompt text, error messages, etc.",
	}, makeSearchEvents(searcher))
}

func makeSearchEvents(s meili.Searcher) mcp.ToolHandlerFor[SearchEventsInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in SearchEventsInput) (*mcp.CallToolResult, any, error) {
		resolved, errRes := resolveCommon(ctx, s, commonInputs{in.SessionID, in.DateRange})
		if errRes != nil {
			return errRes, nil, nil
		}

		hits, total, err := s.SearchEvents(ctx, meili.EventSearchOpts{
			Query:       in.Query,
			ProjectName: in.ProjectName,
			SessionID:   resolved.SessionID,
			HookType:    in.HookType,
			ToolName:    in.ToolName,
			DateRange:   resolved.DateRange,
			Limit:       int64(in.Limit),
		})
		if err != nil {
			return errResult(err.Error()), nil, nil
		}

		if len(hits) == 0 {
			return textResult("No events found."), nil, nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Events matching %q (%d total):\n\n", in.Query, total)

		for _, h := range hits {
			ts := format.FormatTimestamp(h.TimestampUnix)
			sid := format.ShortID(h.SessionID)

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
				detail = format.TruncatePrompt(h.Prompt, 80)
			case "Stop", "SubagentStop":
				detail = fmt.Sprintf("cost=%s tokens=%s/%s", format.FormatCost(h.CostUSD),
					format.FormatTokens(h.InputTokens), format.FormatTokens(h.OutputTokens))
			default:
				detail = h.HookType
			}

			fmt.Fprintf(&b, "[%s] %s  %-20s  %s\n", ts, sid, h.HookType, detail)
		}

		return textResult(b.String()), nil, nil
	}
}
