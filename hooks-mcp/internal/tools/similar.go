package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hooks-mcp/internal/format"
	"hooks-mcp/internal/meili"
)

type SimilarSessionsInput struct {
	SessionID   string `json:"session_id"             jsonschema:"required,description=Session ID (or prefix) to find similar sessions for"`
	ProjectName string `json:"project_name,omitempty" jsonschema:"description=Filter to sessions in this project"`
	DateRange   string `json:"date_range,omitempty"   jsonschema:"description=Date range to search within"`
	Limit       int    `json:"limit,omitempty"        jsonschema:"description=Max results (default 5, max 20)"`
}

func registerSimilarSessions(server *mcp.Server, searcher meili.Searcher) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "similar-sessions",
		Description: "Find sessions similar to a given session. Uses prompt embeddings to identify sessions with similar work patterns and topics. Useful for finding related past work.",
	}, makeSimilarSessions(searcher))
}

func makeSimilarSessions(s meili.Searcher) mcp.ToolHandlerFor[SimilarSessionsInput, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in SimilarSessionsInput) (*mcp.CallToolResult, any, error) {
		// Resolve the source session.
		sessionID, err := s.ResolveSessionPrefix(ctx, in.SessionID)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}

		// Get prompts from the source session to build a representative query.
		prompts, _, err := s.SearchPrompts(ctx, meili.PromptSearchOpts{
			SessionID: sessionID,
			Limit:     10,
		})
		if err != nil {
			return errResult(fmt.Sprintf("fetch session prompts: %v", err)), nil, nil
		}
		if len(prompts) == 0 {
			return textResult("No prompts found in the specified session. Cannot determine session similarity without prompts."), nil, nil
		}

		// Build a composite query from the session's prompts.
		var queryParts []string
		for _, p := range prompts {
			text := format.TruncatePrompt(p.Prompt, 200)
			queryParts = append(queryParts, text)
		}
		compositeQuery := strings.Join(queryParts, " ")
		// Truncate to reasonable length for embedding (rune-safe).
		if len(compositeQuery) > 1000 {
			cut := 1000
			for cut > 0 && !utf8.RuneStart(compositeQuery[cut]) {
				cut--
			}
			compositeQuery = compositeQuery[:cut]
		}

		limit := in.Limit
		if limit <= 0 {
			limit = 5
		}
		if limit > 20 {
			limit = 20
		}

		resolved, errRes := resolveCommon(ctx, s, commonInputs{"", in.DateRange})
		if errRes != nil {
			return errRes, nil, nil
		}

		// Search for semantically similar prompts across other sessions.
		opts := meili.PromptSearchOpts{
			ProjectName: in.ProjectName,
			DateRange:   resolved.DateRange,
			Limit:       int64(limit * 5), // fetch more to deduplicate sessions
		}

		hits, err := s.SemanticSearchPrompts(ctx, compositeQuery, opts)
		if err != nil {
			return errResult(fmt.Sprintf("semantic search: %v", err)), nil, nil
		}

		// Group by session, skip the source session, rank by number of matching prompts.
		type sessionMatch struct {
			sessionID   string
			projectName string
			matchCount  int
			samplePrompt string
			timestamp   int64
		}
		seen := map[string]*sessionMatch{}
		for _, h := range hits {
			if h.SessionID == sessionID {
				continue // skip source session
			}
			if m, ok := seen[h.SessionID]; ok {
				m.matchCount++
			} else {
				seen[h.SessionID] = &sessionMatch{
					sessionID:   h.SessionID,
					projectName: h.ProjectName,
					matchCount:  1,
					samplePrompt: format.TruncatePrompt(h.Prompt, 100),
					timestamp:   h.TimestampUnix,
				}
			}
		}

		if len(seen) == 0 {
			return textResult("No similar sessions found. Ensure embed-svc is running and events have dense embeddings."), nil, nil
		}

		// Sort by match count descending, then by timestamp descending.
		matches := make([]*sessionMatch, 0, len(seen))
		for _, m := range seen {
			matches = append(matches, m)
		}
		// Sort by matchCount desc, then timestamp desc.
		sort.Slice(matches, func(i, j int) bool {
			if matches[i].matchCount != matches[j].matchCount {
				return matches[i].matchCount > matches[j].matchCount
			}
			return matches[i].timestamp > matches[j].timestamp
		})

		if len(matches) > limit {
			matches = matches[:limit]
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Sessions similar to %s (%d found):\n\n", format.ShortID(sessionID), len(matches))

		for i, m := range matches {
			sid := format.ShortID(m.sessionID)
			project := ""
			if m.projectName != "" {
				project = " [" + m.projectName + "]"
			}
			fmt.Fprintf(&b, "%d. %s%s  (%d matching prompts)\n", i+1, sid, project, m.matchCount)
			fmt.Fprintf(&b, "   Sample: %s\n", m.samplePrompt)
		}

		return textResult(b.String()), nil, nil
	}
}
