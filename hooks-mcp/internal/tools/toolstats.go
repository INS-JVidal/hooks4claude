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

type ToolUsageInput struct {
	ProjectName string `json:"project_name,omitempty" jsonschema:"description=Filter by project name"`
	SessionID   string `json:"session_id,omitempty"   jsonschema:"description=Session ID or prefix"`
	DateRange   string `json:"date_range,omitempty"   jsonschema:"description=Date range"`
}

func registerToolUsage(server *mcp.Server, searcher meili.Searcher) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "tool-usage",
		Description: "Tool distribution and file access patterns. Shows tool distribution with bar chart, most-read files, most-written files.",
	}, makeToolUsage(searcher))
}

func makeToolUsage(s meili.Searcher) mcp.ToolHandlerFor[ToolUsageInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in ToolUsageInput) (*mcp.CallToolResult, any, error) {
		resolved, errRes := resolveCommon(ctx, s, commonInputs{in.SessionID, in.DateRange})
		if errRes != nil {
			return errRes, nil, nil
		}

		// Use faceting to get tool distribution efficiently.
		filterOpts := meili.EventSearchOpts{
			ProjectName: in.ProjectName,
			SessionID:   resolved.SessionID,
			HookType:    "PreToolUse",
			DateRange:   resolved.DateRange,
		}
		filter := meili.BuildEventFilter(filterOpts)

		facets, err := s.FacetEvents(ctx, filter, []string{"tool_name"})
		if err != nil {
			return errResult(err.Error()), nil, nil
		}

		var b strings.Builder
		if facets.Approximate {
			fmt.Fprintf(&b, "Tool Usage (%d+ total tool calls, counts approximate):\n\n", facets.TotalHits)
		} else {
			fmt.Fprintf(&b, "Tool Usage (%d total tool calls):\n\n", facets.TotalHits)
		}

		// Tool distribution bar chart.
		if toolDist, ok := facets.Distribution["tool_name"]; ok && len(toolDist) > 0 {
			type toolEntry struct {
				name  string
				count int
			}
			var entries []toolEntry
			for name, count := range toolDist {
				entries = append(entries, toolEntry{name, count})
			}
			sort.Slice(entries, func(i, j int) bool { return entries[i].count > entries[j].count })

			items := make([]format.BarItem, 0, len(entries))
			for _, e := range entries {
				items = append(items, format.BarItem{Label: e.name, Value: e.count})
			}
			b.WriteString("Tool Distribution:\n")
			b.WriteString(format.BarChart(items, 30))
		}

		// Fetch sessions to get file lists.
		if resolved.SessionID != "" {
			// Single session — get file details.
			sessions, _, err := s.SearchSessions(ctx, meili.SessionSearchOpts{
				SessionID: resolved.SessionID,
				Limit:     1,
			})
			if err != nil {
				return errResult(fmt.Sprintf("fetching sessions: %s", err)), nil, nil
			}
			if len(sessions) > 0 {
				sess := sessions[0]
				if len(sess.FilesRead) > 0 {
					fmt.Fprintf(&b, "\nMost-Read Files (%d unique):\n", len(sess.FilesRead))
					showCount := 15
					if len(sess.FilesRead) < showCount {
						showCount = len(sess.FilesRead)
					}
					for _, f := range sess.FilesRead[:showCount] {
						fmt.Fprintf(&b, "  %s\n", f)
					}
				}
				if len(sess.FilesWritten) > 0 {
					fmt.Fprintf(&b, "\nMost-Written Files (%d unique):\n", len(sess.FilesWritten))
					showCount := 15
					if len(sess.FilesWritten) < showCount {
						showCount = len(sess.FilesWritten)
					}
					for _, f := range sess.FilesWritten[:showCount] {
						fmt.Fprintf(&b, "  %s\n", f)
					}
				}
			}
		} else {
			// Multiple sessions — aggregate file lists.
			sessions, _, err := s.SearchSessions(ctx, meili.SessionSearchOpts{
				ProjectName: in.ProjectName,
				DateRange:   resolved.DateRange,
				Limit:       100,
			})
			if err != nil {
				return errResult(fmt.Sprintf("fetching sessions: %s", err)), nil, nil
			}
			if len(sessions) > 0 {
				readCounts := make(map[string]int)
				writeCounts := make(map[string]int)
				for _, sess := range sessions {
					for _, f := range sess.FilesRead {
						readCounts[f]++
					}
					for _, f := range sess.FilesWritten {
						writeCounts[f]++
					}
				}
				b.WriteString(topFiles("\nMost-Read Files", readCounts, 10))
				b.WriteString(topFiles("\nMost-Written Files", writeCounts, 10))
			}
		}

		return textResult(b.String()), nil, nil
	}
}

func topFiles(title string, counts map[string]int, n int) string {
	if len(counts) == 0 {
		return ""
	}

	type entry struct {
		file  string
		count int
	}
	var entries []entry
	for f, c := range counts {
		entries = append(entries, entry{f, c})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].count > entries[j].count })

	if len(entries) > n {
		entries = entries[:n]
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s (%d unique):\n", title, len(counts))
	for _, e := range entries {
		fmt.Fprintf(&b, "  %3d× %s\n", e.count, e.file)
	}
	return b.String()
}
