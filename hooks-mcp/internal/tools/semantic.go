package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hooks-mcp/internal/format"
	"hooks-mcp/internal/meili"
)

type SemanticSearchInput struct {
	Query       string `json:"query"                  jsonschema:"required,description=Natural language query to find semantically similar past events or prompts"`
	SearchType  string `json:"search_type,omitempty"   jsonschema:"description=What to search: events (default) or prompts"`
	ProjectName string `json:"project_name,omitempty" jsonschema:"description=Filter by project name"`
	SessionID   string `json:"session_id,omitempty"   jsonschema:"description=Session ID or prefix"`
	HookType    string `json:"hook_type,omitempty"    jsonschema:"description=Filter by hook type"`
	DateRange   string `json:"date_range,omitempty"   jsonschema:"description=Date range"`
	Limit       int    `json:"limit,omitempty"        jsonschema:"description=Max results (default 20, max 100)"`
}

func registerSemanticSearch(server *mcp.Server, searcher meili.Searcher) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "semantic-search",
		Description: "Find past events or prompts semantically similar to a natural language query. Uses dense vector embeddings — finds matches by meaning, not just keywords. Example: 'fix authentication bug' will find 'resolve login issue'.",
	}, makeSemanticSearch(searcher))
}

func makeSemanticSearch(s meili.Searcher) mcp.ToolHandlerFor[SemanticSearchInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in SemanticSearchInput) (*mcp.CallToolResult, any, error) {
		resolved, errRes := resolveCommon(ctx, s, commonInputs{in.SessionID, in.DateRange})
		if errRes != nil {
			return errRes, nil, nil
		}

		searchType := in.SearchType
		if searchType == "" {
			searchType = "events"
		}

		switch searchType {
		case "prompts":
			return semanticSearchPrompts(ctx, s, in, resolved)
		case "events":
			return semanticSearchEvents(ctx, s, in, resolved)
		default:
			return errResult(fmt.Sprintf("unknown search_type %q (use 'events' or 'prompts')", searchType)), nil, nil
		}
	}
}

func semanticSearchPrompts(ctx context.Context, s meili.Searcher, in SemanticSearchInput, resolved resolvedInputs) (*mcp.CallToolResult, any, error) {
	opts := meili.PromptSearchOpts{
		ProjectName: in.ProjectName,
		SessionID:   resolved.SessionID,
		DateRange:   resolved.DateRange,
		Limit:       int64(in.Limit),
	}

	hits, err := s.SemanticSearchPrompts(ctx, in.Query, opts)
	if err != nil {
		return errResult(err.Error()), nil, nil
	}
	if len(hits) == 0 {
		return textResult("No semantically similar prompts found. Ensure embed-svc is running and prompts have dense embeddings."), nil, nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Prompts similar to %q (%d results):\n\n", in.Query, len(hits))

	for i, h := range hits {
		ts := format.FormatTimestamp(h.TimestampUnix)
		sid := format.ShortID(h.SessionID)
		prompt := format.TruncatePrompt(h.Prompt, 120)
		fmt.Fprintf(&b, "%d. [%s] %s  %s\n", i+1, ts, sid, prompt)
		if h.ProjectName != "" {
			fmt.Fprintf(&b, "   project: %s\n", h.ProjectName)
		}
	}

	return textResult(b.String()), nil, nil
}

func semanticSearchEvents(ctx context.Context, s meili.Searcher, in SemanticSearchInput, resolved resolvedInputs) (*mcp.CallToolResult, any, error) {
	opts := meili.EventSearchOpts{
		ProjectName: in.ProjectName,
		SessionID:   resolved.SessionID,
		HookType:    in.HookType,
		DateRange:   resolved.DateRange,
		Limit:       int64(in.Limit),
	}

	hits, err := s.SemanticSearchEvents(ctx, in.Query, opts)
	if err != nil {
		return errResult(err.Error()), nil, nil
	}
	if len(hits) == 0 {
		return textResult("No semantically similar events found. Ensure embed-svc is running and events have dense embeddings."), nil, nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Events similar to %q (%d results):\n\n", in.Query, len(hits))

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
