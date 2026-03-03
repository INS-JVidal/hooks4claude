package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hooks-mcp/internal/format"
	"hooks-mcp/internal/meili"
)

type SessionSummaryInput struct {
	SessionID string `json:"session_id" jsonschema:"required,description=Session ID or prefix (e.g. af7deb64)"`
}

func registerSessionSummary(server *mcp.Server, searcher meili.Searcher) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "session-summary",
		Description: "Detailed overview of one Claude session: metadata, tool breakdown, files read/written, errors, prompt list, cost/tokens.",
	}, makeSessionSummary(searcher))
}

func makeSessionSummary(s meili.Searcher) mcp.ToolHandlerFor[SessionSummaryInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in SessionSummaryInput) (*mcp.CallToolResult, any, error) {
		sessionID, err := s.ResolveSessionPrefix(ctx, in.SessionID)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}

		// Fetch session summary from sessions index.
		sessions, _, err := s.SearchSessions(ctx, meili.SessionSearchOpts{
			Query: sessionID,
			Limit: 1,
		})
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		if len(sessions) == 0 {
			return errResult(fmt.Sprintf("Session %s not found", sessionID)), nil, nil
		}
		sess := sessions[0]

		// Fetch prompts for this session.
		prompts, _, err := s.SearchPrompts(ctx, meili.PromptSearchOpts{
			SessionID: sessionID,
			Limit:     100,
		})
		if err != nil {
			return errResult(fmt.Sprintf("fetching prompts: %s", err)), nil, nil
		}

		// Fetch errors for this session.
		errors, _, err := s.SearchEvents(ctx, meili.EventSearchOpts{
			SessionID: sessionID,
			HookType:  "PostToolUseFailure",
			Limit:     20,
		})
		if err != nil {
			return errResult(fmt.Sprintf("fetching errors: %s", err)), nil, nil
		}

		var b strings.Builder

		// Header.
		fmt.Fprintf(&b, "Session: %s\n", sessionID)
		fmt.Fprintf(&b, "Started: %s  Duration: %s\n", format.FormatTimestampISO(sess.StartedAt), format.FormatDuration(sess.DurationS))
		fmt.Fprintf(&b, "Project: %s  Model: %s  Source: %s\n", sess.ProjectName, sess.Model, sess.Source)
		fmt.Fprintf(&b, "Events: %d  Prompts: %d  Compactions: %d\n", sess.TotalEvents, sess.TotalPrompts, sess.CompactionCount)
		fmt.Fprintf(&b, "Cost: %s  Input: %s  Output: %s\n\n",
			format.FormatCost(sess.TotalCostUSD),
			format.FormatTokens(sess.InputTokens),
			format.FormatTokens(sess.OutputTokens))

		// Tool breakdown.
		fmt.Fprintf(&b, "Tool Usage:\n")
		toolItems := []format.BarItem{}
		addTool := func(name string, count int) {
			if count > 0 {
				toolItems = append(toolItems, format.BarItem{Label: name, Value: count})
			}
		}
		addTool("Read", sess.ReadCount)
		addTool("Edit", sess.EditCount)
		addTool("Write", sess.WriteCount)
		addTool("Bash", sess.BashCount)
		addTool("Grep", sess.GrepCount)
		addTool("Glob", sess.GlobCount)
		addTool("Agent", sess.AgentCount)
		addTool("Other", sess.OtherToolCount)
		if len(toolItems) > 0 {
			b.WriteString(format.BarChart(toolItems, 30))
		} else {
			b.WriteString("  (no tool usage recorded)\n")
		}

		// Files.
		if len(sess.FilesRead) > 0 {
			fmt.Fprintf(&b, "\nFiles Read (%d):\n", len(sess.FilesRead))
			for _, f := range sess.FilesRead {
				fmt.Fprintf(&b, "  %s\n", f)
			}
		}
		if len(sess.FilesWritten) > 0 {
			fmt.Fprintf(&b, "\nFiles Written (%d):\n", len(sess.FilesWritten))
			for _, f := range sess.FilesWritten {
				fmt.Fprintf(&b, "  %s\n", f)
			}
		}

		// Prompts.
		if len(prompts) > 0 {
			fmt.Fprintf(&b, "\nPrompts (%d):\n", len(prompts))
			for i, p := range prompts {
				ts := format.FormatTimestamp(p.TimestampUnix)
				prompt := format.TruncatePrompt(p.Prompt, 120)
				fmt.Fprintf(&b, "  %d. [%s] %s\n", i+1, ts, prompt)
			}
		}

		// Errors.
		if len(errors) > 0 {
			fmt.Fprintf(&b, "\nErrors (%d):\n", len(errors))
			for _, e := range errors {
				ts := format.FormatTimestamp(e.TimestampUnix)
				fmt.Fprintf(&b, "  [%s] %s: %s\n", ts, e.ToolName, format.TruncatePrompt(e.ErrorMessage, 100))
			}
		}

		return textResult(b.String()), nil, nil
	}
}
