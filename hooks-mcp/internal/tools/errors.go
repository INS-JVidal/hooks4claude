package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hooks-mcp/internal/format"
	"hooks-mcp/internal/meili"
)

type ErrorAnalysisInput struct {
	ProjectName string `json:"project_name,omitempty" jsonschema:"description=Filter by project name"`
	SessionID   string `json:"session_id,omitempty"   jsonschema:"description=Session ID or prefix"`
	DateRange   string `json:"date_range,omitempty"   jsonschema:"description=Date range"`
	Limit       int    `json:"limit,omitempty"        jsonschema:"description=Max results (default 50)"`
}

func registerErrorAnalysis(server *mcp.Server, searcher meili.Searcher) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "error-analysis",
		Description: "Analyze PostToolUseFailure events: error frequency by tool, common patterns, recent errors list.",
	}, makeErrorAnalysis(searcher))
}

func makeErrorAnalysis(s meili.Searcher) mcp.ToolHandlerFor[ErrorAnalysisInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in ErrorAnalysisInput) (*mcp.CallToolResult, any, error) {
		resolved, errRes := resolveCommon(ctx, s, commonInputs{in.SessionID, in.DateRange})
		if errRes != nil {
			return errRes, nil, nil
		}

		limit := int64(in.Limit)
		if limit <= 0 {
			limit = 50
		}

		hits, total, err := s.SearchEvents(ctx, meili.EventSearchOpts{
			ProjectName: in.ProjectName,
			SessionID:   resolved.SessionID,
			HookType:    "PostToolUseFailure",
			DateRange:   resolved.DateRange,
			Sort:        []string{"timestamp_unix:desc"},
			Limit:       limit,
		})
		if err != nil {
			return errResult(err.Error()), nil, nil
		}

		if len(hits) == 0 {
			return textResult("No errors found."), nil, nil
		}

		// Count errors by tool.
		toolCounts := make(map[string]int)
		for _, h := range hits {
			toolCounts[h.ToolName]++
		}

		type toolCount struct {
			name  string
			count int
		}
		var sorted []toolCount
		for name, count := range toolCounts {
			sorted = append(sorted, toolCount{name, count})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })

		var b strings.Builder
		fmt.Fprintf(&b, "Error Analysis (%d total errors):\n\n", total)

		// Error frequency by tool.
		fmt.Fprintf(&b, "Errors by Tool:\n")
		barItems := make([]format.BarItem, 0, len(sorted))
		for _, tc := range sorted {
			barItems = append(barItems, format.BarItem{Label: tc.name, Value: tc.count})
		}
		b.WriteString(format.BarChart(barItems, 30))

		// Recent errors list.
		fmt.Fprintf(&b, "\nRecent Errors:\n")
		showCount := len(hits)
		if showCount > 20 {
			showCount = 20
		}
		for _, h := range hits[:showCount] {
			ts := format.FormatTimestamp(h.TimestampUnix)
			sid := format.ShortID(h.SessionID)
			msg := format.TruncatePrompt(h.ErrorMessage, 100)
			fmt.Fprintf(&b, "  [%s] %s  %s: %s\n", ts, sid, h.ToolName, msg)
		}

		return textResult(b.String()), nil, nil
	}
}
