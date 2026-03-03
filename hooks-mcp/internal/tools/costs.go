package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hooks-mcp/internal/dateparse"
	"hooks-mcp/internal/format"
	"hooks-mcp/internal/meili"
)

type CostAnalysisInput struct {
	ProjectName string `json:"project_name,omitempty" jsonschema:"description=Filter by project name"`
	DateRange   string `json:"date_range,omitempty"   jsonschema:"description=Date range (default: last 7 days)"`
	GroupBy     string `json:"group_by,omitempty"     jsonschema:"description=Group by: day (default), session, model"`
}

func registerCostAnalysis(server *mcp.Server, searcher meili.Searcher) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "cost-analysis",
		Description: "Cost and token usage analysis. Shows totals, averages, grouped breakdown, and most expensive sessions.",
	}, makeCostAnalysis(searcher))
}

func makeCostAnalysis(s meili.Searcher) mcp.ToolHandlerFor[CostAnalysisInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in CostAnalysisInput) (*mcp.CallToolResult, any, error) {
		drStr := in.DateRange
		if drStr == "" {
			drStr = "last 7 days"
		}
		dr, err := dateparse.ParseRange(drStr, time.Now())
		if err != nil {
			return errResult(err.Error()), nil, nil
		}

		// Fetch sessions with cost data.
		hits, total, err := s.SearchSessions(ctx, meili.SessionSearchOpts{
			ProjectName: in.ProjectName,
			DateRange:   dr,
			SortBy:      "total_cost_usd",
			Limit:       100,
		})
		if err != nil {
			return errResult(err.Error()), nil, nil
		}

		if len(hits) == 0 {
			return textResult("No sessions found."), nil, nil
		}

		// Compute totals.
		var totalCost float64
		var totalInput, totalOutput int64
		for _, h := range hits {
			totalCost += h.TotalCostUSD
			totalInput += h.InputTokens
			totalOutput += h.OutputTokens
		}
		avgCost := totalCost / float64(len(hits))

		var b strings.Builder
		fmt.Fprintf(&b, "Cost Analysis (%d sessions, %d total):\n\n", len(hits), total)
		fmt.Fprintf(&b, "Total Cost:    %s\n", format.FormatCost(totalCost))
		fmt.Fprintf(&b, "Average Cost:  %s per session\n", format.FormatCost(avgCost))
		fmt.Fprintf(&b, "Input Tokens:  %s\n", format.FormatTokens(totalInput))
		fmt.Fprintf(&b, "Output Tokens: %s\n\n", format.FormatTokens(totalOutput))

		groupBy := in.GroupBy
		if groupBy == "" {
			groupBy = "day"
		}

		switch groupBy {
		case "day":
			b.WriteString(groupByDay(hits))
		case "session":
			b.WriteString(groupBySession(hits))
		case "model":
			b.WriteString(groupByModel(hits))
		default:
			return errResult(fmt.Sprintf("invalid group_by %q: use day, session, or model", groupBy)), nil, nil
		}

		// Most expensive sessions.
		fmt.Fprintf(&b, "\nMost Expensive Sessions:\n")
		sorted := make([]meili.SessionHit, len(hits))
		copy(sorted, hits)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].TotalCostUSD > sorted[j].TotalCostUSD })
		showCount := 5
		if len(sorted) < showCount {
			showCount = len(sorted)
		}
		headers := []string{"ID", "Date", "Cost", "Duration", "Prompts", "Model"}
		rows := make([][]string, 0, showCount)
		for _, h := range sorted[:showCount] {
			rows = append(rows, []string{
				format.ShortID(h.SessionID),
				format.FormatTimestampISO(h.StartedAt),
				format.FormatCost(h.TotalCostUSD),
				format.FormatDuration(h.DurationS),
				fmt.Sprintf("%d", h.TotalPrompts),
				h.Model,
			})
		}
		b.WriteString(format.Table(headers, rows))

		return textResult(b.String()), nil, nil
	}
}

func groupByDay(hits []meili.SessionHit) string {
	dayMap := make(map[string]struct {
		cost  float64
		count int
	})
	var days []string

	for _, h := range hits {
		date := format.FormatTimestampISO(h.StartedAt)
		if len(date) >= 10 {
			date = date[:10]
		}
		entry := dayMap[date]
		if entry.count == 0 {
			days = append(days, date)
		}
		entry.cost += h.TotalCostUSD
		entry.count++
		dayMap[date] = entry
	}

	sort.Sort(sort.Reverse(sort.StringSlice(days)))

	var b strings.Builder
	b.WriteString("Cost by Day:\n")
	headers := []string{"Date", "Sessions", "Cost"}
	rows := make([][]string, 0, len(days))
	for _, d := range days {
		entry := dayMap[d]
		rows = append(rows, []string{d, fmt.Sprintf("%d", entry.count), format.FormatCost(entry.cost)})
	}
	b.WriteString(format.Table(headers, rows))
	return b.String()
}

func groupBySession(hits []meili.SessionHit) string {
	var b strings.Builder
	b.WriteString("Cost by Session:\n")
	headers := []string{"ID", "Date", "Cost", "Input", "Output"}
	rows := make([][]string, 0, len(hits))
	for _, h := range hits {
		rows = append(rows, []string{
			format.ShortID(h.SessionID),
			format.FormatTimestampISO(h.StartedAt),
			format.FormatCost(h.TotalCostUSD),
			format.FormatTokens(h.InputTokens),
			format.FormatTokens(h.OutputTokens),
		})
	}
	b.WriteString(format.Table(headers, rows))
	return b.String()
}

func groupByModel(hits []meili.SessionHit) string {
	type modelEntry struct {
		model string
		cost  float64
		count int
	}
	modelMap := make(map[string]*modelEntry)
	for _, h := range hits {
		model := h.Model
		if model == "" {
			model = "(unknown)"
		}
		e := modelMap[model]
		if e == nil {
			e = &modelEntry{model: model}
			modelMap[model] = e
		}
		e.cost += h.TotalCostUSD
		e.count++
	}

	entries := make([]modelEntry, 0, len(modelMap))
	for _, e := range modelMap {
		entries = append(entries, *e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].cost > entries[j].cost })

	var b strings.Builder
	b.WriteString("Cost by Model:\n")
	headers := []string{"Model", "Sessions", "Cost"}
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, []string{e.model, fmt.Sprintf("%d", e.count), format.FormatCost(e.cost)})
	}
	b.WriteString(format.Table(headers, rows))
	return b.String()
}
