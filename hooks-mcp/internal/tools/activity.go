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

type ProjectActivityInput struct {
	ProjectName string `json:"project_name" jsonschema:"required,description=Project name (e.g. hooks4claude)"`
	DateRange   string `json:"date_range,omitempty" jsonschema:"description=Date range (default: last 7 days)"`
}

func registerProjectActivity(server *mcp.Server, searcher meili.Searcher) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "project-activity",
		Description: "Activity tree showing sessions grouped by day and submodule. Classifies sessions by files touched (not cwd).",
	}, makeProjectActivity(searcher))
}

func makeProjectActivity(s meili.Searcher) mcp.ToolHandlerFor[ProjectActivityInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in ProjectActivityInput) (*mcp.CallToolResult, any, error) {
		drStr := in.DateRange
		if drStr == "" {
			drStr = "last 7 days"
		}
		dr, err := dateparse.ParseRange(drStr, time.Now())
		if err != nil {
			return errResult(err.Error()), nil, nil
		}

		hits, _, err := s.SearchSessions(ctx, meili.SessionSearchOpts{
			ProjectName: in.ProjectName,
			DateRange:   dr,
			SortBy:      "started_at",
			Limit:       100,
		})
		if err != nil {
			return errResult(err.Error()), nil, nil
		}

		if len(hits) == 0 {
			return textResult("No sessions found."), nil, nil
		}

		// Group sessions by day, then classify by submodule.
		type dayGroup struct {
			date     string
			sessions []meili.SessionHit
		}
		dayMap := make(map[string]*dayGroup)
		var dayOrder []string

		for _, h := range hits {
			date := format.FormatTimestampISO(h.StartedAt)
			if len(date) >= 10 {
				date = date[:10]
			}
			if _, ok := dayMap[date]; !ok {
				dayMap[date] = &dayGroup{date: date}
				dayOrder = append(dayOrder, date)
			}
			dayMap[date].sessions = append(dayMap[date].sessions, h)
		}

		sort.Sort(sort.Reverse(sort.StringSlice(dayOrder)))

		// Build tree.
		roots := make([]format.TreeNode, 0, len(dayOrder))
		for _, date := range dayOrder {
			dg := dayMap[date]
			// Classify sessions by submodule.
			subMap := make(map[string][]meili.SessionHit)
			for _, sess := range dg.sessions {
				sub := classifySubmodule(sess)
				subMap[sub] = append(subMap[sub], sess)
			}

			var subNames []string
			for name := range subMap {
				subNames = append(subNames, name)
			}
			sort.Strings(subNames)

			dayNode := format.TreeNode{Label: fmt.Sprintf("%s (%d sessions)", date, len(dg.sessions))}
			for _, subName := range subNames {
				subNode := format.TreeNode{Label: subName}
				for _, sess := range subMap[subName] {
					label := fmt.Sprintf("%s  %s  %s  %s",
						format.ShortID(sess.SessionID),
						format.FormatDuration(sess.DurationS),
						format.FormatCost(sess.TotalCostUSD),
						format.TruncatePrompt(sess.PromptPreview, 40))
					subNode.Children = append(subNode.Children, format.TreeNode{Label: label})
				}
				dayNode.Children = append(dayNode.Children, subNode)
			}
			roots = append(roots, dayNode)
		}

		result := fmt.Sprintf("Project: %s\n\n%s", in.ProjectName, format.Tree(roots))
		return textResult(result), nil, nil
	}
}

// classifySubmodule determines which submodule a session belongs to
// based on files_read and files_written paths. Sessions touching files
// under a submodule directory are classified to that submodule.
func classifySubmodule(sess meili.SessionHit) string {
	subs := make(map[string]bool)
	allFiles := append(sess.FilesRead, sess.FilesWritten...)
	for _, f := range allFiles {
		for _, sub := range []string{"claude-hooks-monitor", "hooks-store", "hooks-mcp"} {
			if strings.Contains(f, sub+"/") || strings.Contains(f, sub+"\\") {
				subs[sub] = true
			}
		}
	}

	switch len(subs) {
	case 0:
		return "parent"
	case 1:
		for name := range subs {
			return name
		}
	}
	return "cross"
}
