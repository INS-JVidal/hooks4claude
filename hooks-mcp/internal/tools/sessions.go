package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hooks-mcp/internal/dateparse"
	"hooks-mcp/internal/format"
	"hooks-mcp/internal/meili"
)

type QuerySessionsInput struct {
	ProjectName string `json:"project_name,omitempty" jsonschema:"description=Filter by project name (e.g. hooks4claude)"`
	DateRange   string `json:"date_range,omitempty"   jsonschema:"description=Date range: today, yesterday, last N days, last N hours, YYYY-MM-DD, YYYY-MM-DD..YYYY-MM-DD"`
	Model       string `json:"model,omitempty"        jsonschema:"description=Filter by model name"`
	SortBy      string `json:"sort_by,omitempty"      jsonschema:"description=Sort by: started_at (default), duration_s, total_cost_usd, total_events"`
	Limit       int    `json:"limit,omitempty"        jsonschema:"description=Max results (default 20, max 100)"`
}

func registerQuerySessions(server *mcp.Server, searcher meili.Searcher) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "query-sessions",
		Description: "List Claude sessions filtered by project, date, model. Returns a table with session ID, date, duration, prompts, events, cost, model, and first prompt preview.",
	}, makeQuerySessions(searcher))
}

func makeQuerySessions(s meili.Searcher) mcp.ToolHandlerFor[QuerySessionsInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in QuerySessionsInput) (*mcp.CallToolResult, any, error) {
		dr, err := dateparse.ParseRange(in.DateRange, time.Now())
		if err != nil {
			return errResult(err.Error()), nil, nil
		}

		if in.SortBy != "" {
			valid := map[string]bool{"started_at": true, "duration_s": true, "total_cost_usd": true, "total_events": true}
			if !valid[in.SortBy] {
				return errResult(fmt.Sprintf("invalid sort_by %q: use started_at, duration_s, total_cost_usd, or total_events", in.SortBy)), nil, nil
			}
		}

		hits, total, err := s.SearchSessions(ctx, meili.SessionSearchOpts{
			ProjectName: in.ProjectName,
			Model:       in.Model,
			DateRange:   dr,
			SortBy:      in.SortBy,
			Limit:       int64(in.Limit),
		})
		if err != nil {
			return errResult(err.Error()), nil, nil
		}

		if len(hits) == 0 {
			return textResult("No sessions found."), nil, nil
		}

		headers := []string{"ID", "Date", "Duration", "Prompts", "Events", "Cost", "Model", "Preview"}
		rows := make([][]string, 0, len(hits))
		for _, h := range hits {
			rows = append(rows, []string{
				format.ShortID(h.SessionID),
				format.FormatTimestampISO(h.StartedAt),
				format.FormatDuration(h.DurationS),
				fmt.Sprintf("%d", h.TotalPrompts),
				fmt.Sprintf("%d", h.TotalEvents),
				format.FormatCost(h.TotalCostUSD),
				h.Model,
				format.TruncatePrompt(h.PromptPreview, 50),
			})
		}

		result := fmt.Sprintf("Sessions (%d total):\n\n%s", total, format.Table(headers, rows))
		return textResult(result), nil, nil
	}
}
