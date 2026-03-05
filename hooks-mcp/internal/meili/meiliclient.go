//go:build meili

// MeiliClient is the original MeiliSearch implementation of Searcher.
// Preserved for reference. Build with -tags=meili to compile.
package meili

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/meilisearch/meilisearch-go"

	"hooks-mcp/internal/dateparse"
)

// Searcher is the interface tools use to query MeiliSearch.
// Implementations must be safe for concurrent use.
type Searcher interface {
	SearchSessions(ctx context.Context, opts SessionSearchOpts) ([]SessionHit, int64, error)
	SearchPrompts(ctx context.Context, opts PromptSearchOpts) ([]PromptHit, int64, error)
	SearchEvents(ctx context.Context, opts EventSearchOpts) ([]EventHit, int64, error)
	FacetEvents(ctx context.Context, filter string, facets []string) (*FacetResult, error)
	ResolveSessionPrefix(ctx context.Context, prefix string) (string, error)
	SemanticSearchPrompts(ctx context.Context, queryText string, opts PromptSearchOpts) ([]PromptHit, error)
	SemanticSearchEvents(ctx context.Context, queryText string, opts EventSearchOpts) ([]EventHit, error)
	HybridSearch(ctx context.Context, queryText string, opts EventSearchOpts) ([]EventHit, error)
}

// SessionSearchOpts controls session queries.
type SessionSearchOpts struct {
	Query       string
	ProjectName string
	Model       string
	Source      string
	DateRange   dateparse.DateRange
	SortBy      string // started_at, duration_s, total_cost_usd, total_events
	Limit       int64
	Offset      int64
}

// PromptSearchOpts controls prompt queries.
type PromptSearchOpts struct {
	Query       string
	ProjectName string
	SessionID   string
	DateRange   dateparse.DateRange
	Limit       int64
	Offset      int64
}

// EventSearchOpts controls event queries.
type EventSearchOpts struct {
	Query       string
	ProjectName string
	SessionID   string
	HookType    string
	ToolName    string
	DateRange   dateparse.DateRange
	Sort        []string
	Limit       int64
	Offset      int64
	Facets      []string
}

// SessionHit mirrors hooks-store/internal/store.SessionDocument.
type SessionHit struct {
	ID              string   `json:"id"`
	SessionID       string   `json:"session_id"`
	ProjectName     string   `json:"project_name"`
	ProjectDir      string   `json:"project_dir"`
	Cwd             string   `json:"cwd"`
	StartedAt       string   `json:"started_at"`
	EndedAt         string   `json:"ended_at"`
	DurationS       float64  `json:"duration_s"`
	Source          string   `json:"source"`
	Model           string   `json:"model"`
	PermissionMode  string   `json:"permission_mode"`
	HasClaudeMD     bool     `json:"has_claude_md"`
	TotalEvents     int      `json:"total_events"`
	TotalPrompts    int      `json:"total_prompts"`
	CompactionCount int      `json:"compaction_count"`
	ReadCount       int      `json:"read_count"`
	EditCount       int      `json:"edit_count"`
	BashCount       int      `json:"bash_count"`
	GrepCount       int      `json:"grep_count"`
	GlobCount       int      `json:"glob_count"`
	WriteCount      int      `json:"write_count"`
	AgentCount      int      `json:"agent_count"`
	OtherToolCount  int      `json:"other_tool_count"`
	TotalCostUSD    float64  `json:"total_cost_usd"`
	InputTokens     int64    `json:"input_tokens"`
	OutputTokens    int64    `json:"output_tokens"`
	PromptPreview   string   `json:"prompt_preview"`
	FilesRead       []string `json:"files_read"`
	FilesWritten    []string `json:"files_written"`
}

// PromptHit mirrors hooks-store/internal/store.PromptDocument.
type PromptHit struct {
	ID             string `json:"id"`
	HookType       string `json:"hook_type"`
	Timestamp      string `json:"timestamp"`
	TimestampUnix  int64  `json:"timestamp_unix"`
	SessionID      string `json:"session_id"`
	Prompt         string `json:"prompt"`
	PromptLength   int    `json:"prompt_length"`
	Cwd            string `json:"cwd"`
	ProjectDir     string `json:"project_dir"`
	ProjectName    string `json:"project_name"`
	PermissionMode string `json:"permission_mode"`
	HasClaudeMD    bool   `json:"has_claude_md"`
}

// EventHit mirrors hooks-store/internal/store.Document.
type EventHit struct {
	ID                string                 `json:"id"`
	HookType          string                 `json:"hook_type"`
	Timestamp         string                 `json:"timestamp"`
	TimestampUnix     int64                  `json:"timestamp_unix"`
	SessionID         string                 `json:"session_id"`
	ToolName          string                 `json:"tool_name"`
	HasClaudeMD       bool                   `json:"has_claude_md"`
	InputTokens       int64                  `json:"input_tokens"`
	OutputTokens      int64                  `json:"output_tokens"`
	CacheReadTokens   int64                  `json:"cache_read_tokens"`
	CacheCreateTokens int64                  `json:"cache_create_tokens"`
	CostUSD           float64                `json:"cost_usd"`
	Prompt            string                 `json:"prompt"`
	FilePath          string                 `json:"file_path"`
	ErrorMessage      string                 `json:"error_message"`
	ProjectDir        string                 `json:"project_dir"`
	ProjectName       string                 `json:"project_name"`
	PermissionMode    string                 `json:"permission_mode"`
	Cwd               string                 `json:"cwd"`
	LastMessage       string                 `json:"last_message"`
	TranscriptPath    string                 `json:"transcript_path"`
	Source            string                 `json:"source"`
	TaskSubject       string                 `json:"task_subject"`
	Model             string                 `json:"model"`
	DataFlat          string                 `json:"data_flat"`
	Data              map[string]interface{} `json:"data"`
}

// FacetResult holds facet distribution data from a search.
type FacetResult struct {
	TotalHits    int64
	Distribution map[string]map[string]int // facet name -> value -> count
}

// MeiliClient implements Searcher using the meilisearch-go SDK.
type MeiliClient struct {
	events   meilisearch.IndexManager
	prompts  meilisearch.IndexManager
	sessions meilisearch.IndexManager
}

// NewMeiliClient creates a MeiliClient from an existing SDK client.
func NewMeiliClient(client meilisearch.ServiceManager, eventsIdx, promptsIdx, sessionsIdx string) *MeiliClient {
	return &MeiliClient{
		events:   client.Index(eventsIdx),
		prompts:  client.Index(promptsIdx),
		sessions: client.Index(sessionsIdx),
	}
}

func (c *MeiliClient) SearchSessions(ctx context.Context, opts SessionSearchOpts) ([]SessionHit, int64, error) {
	filter := buildSessionFilter(opts)
	sort := []string{"started_at:desc"}
	if opts.SortBy != "" {
		sort = []string{opts.SortBy + ":desc"}
	}

	limit := clamp(opts.Limit, 20, 100)

	resp, err := c.sessions.SearchWithContext(ctx, opts.Query, &meilisearch.SearchRequest{
		Filter: filter,
		Sort:   sort,
		Limit:  limit,
		Offset: opts.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("search sessions: %w", err)
	}

	var hits []SessionHit
	if err := resp.Hits.DecodeInto(&hits); err != nil {
		return nil, 0, fmt.Errorf("decode session hits: %w", err)
	}
	return hits, resp.EstimatedTotalHits, nil
}

func (c *MeiliClient) SearchPrompts(ctx context.Context, opts PromptSearchOpts) ([]PromptHit, int64, error) {
	filter := buildPromptFilter(opts)
	limit := clamp(opts.Limit, 50, 200)

	resp, err := c.prompts.SearchWithContext(ctx, opts.Query, &meilisearch.SearchRequest{
		Filter: filter,
		Sort:   []string{"timestamp_unix:asc"},
		Limit:  limit,
		Offset: opts.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("search prompts: %w", err)
	}

	var hits []PromptHit
	if err := resp.Hits.DecodeInto(&hits); err != nil {
		return nil, 0, fmt.Errorf("decode prompt hits: %w", err)
	}
	return hits, resp.EstimatedTotalHits, nil
}

func (c *MeiliClient) SearchEvents(ctx context.Context, opts EventSearchOpts) ([]EventHit, int64, error) {
	filter := buildEventFilter(opts)
	limit := clamp(opts.Limit, 20, 100)

	sort := opts.Sort
	if len(sort) == 0 {
		sort = []string{"timestamp_unix:desc"}
	}

	resp, err := c.events.SearchWithContext(ctx, opts.Query, &meilisearch.SearchRequest{
		Filter: filter,
		Sort:   sort,
		Limit:  limit,
		Offset: opts.Offset,
		Facets: opts.Facets,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("search events: %w", err)
	}

	var hits []EventHit
	if err := resp.Hits.DecodeInto(&hits); err != nil {
		return nil, 0, fmt.Errorf("decode event hits: %w", err)
	}
	return hits, resp.EstimatedTotalHits, nil
}

func (c *MeiliClient) FacetEvents(ctx context.Context, filter string, facets []string) (*FacetResult, error) {
	resp, err := c.events.SearchWithContext(ctx, "", &meilisearch.SearchRequest{
		Filter: filter,
		Facets: facets,
		Limit:  0,
	})
	if err != nil {
		return nil, fmt.Errorf("facet events: %w", err)
	}

	dist := make(map[string]map[string]int)
	if resp.FacetDistribution != nil {
		var rawDist map[string]map[string]float64
		if err := json.Unmarshal(resp.FacetDistribution, &rawDist); err != nil {
			fmt.Fprintf(os.Stderr, "hooks-mcp: warning: decode facet distribution: %v\n", err)
		} else {
			for facetName, facetValues := range rawDist {
				m := make(map[string]int)
				for val, count := range facetValues {
					m[val] = int(count)
				}
				dist[facetName] = m
			}
		}
	}

	return &FacetResult{
		TotalHits:    resp.EstimatedTotalHits,
		Distribution: dist,
	}, nil
}

func (c *MeiliClient) ResolveSessionPrefix(ctx context.Context, prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", fmt.Errorf("empty session ID")
	}

	// Full UUID — return as-is.
	if len(prefix) == 36 {
		return prefix, nil
	}

	// Search sessions index for prefix match.
	resp, err := c.sessions.SearchWithContext(ctx, prefix, &meilisearch.SearchRequest{
		Limit: 10,
	})
	if err != nil {
		return "", fmt.Errorf("search for session prefix %q: %w", prefix, err)
	}

	// Decode hits and confirm prefix match client-side (MeiliSearch is fuzzy).
	var sessions []SessionHit
	if err := resp.Hits.DecodeInto(&sessions); err != nil {
		return "", fmt.Errorf("decode session hits: %w", err)
	}

	var matches []string
	for _, s := range sessions {
		if strings.HasPrefix(s.SessionID, prefix) {
			matches = append(matches, s.SessionID)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no session found matching prefix %q", prefix)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous prefix %q matches %d sessions: %s",
			prefix, len(matches), strings.Join(matches[:min(3, len(matches))], ", "))
	}
}

// Filter builders.

func buildSessionFilter(opts SessionSearchOpts) string {
	var parts []string
	if opts.ProjectName != "" {
		parts = append(parts, fmt.Sprintf("project_name = %q", opts.ProjectName))
	}
	if opts.Model != "" {
		parts = append(parts, fmt.Sprintf("model = %q", opts.Model))
	}
	if opts.Source != "" {
		parts = append(parts, fmt.Sprintf("source = %q", opts.Source))
	}
	if !opts.DateRange.IsZero {
		parts = append(parts, fmt.Sprintf("started_at >= %q", opts.DateRange.StartISO))
		parts = append(parts, fmt.Sprintf("started_at < %q", opts.DateRange.EndISO))
	}
	return strings.Join(parts, " AND ")
}

func buildPromptFilter(opts PromptSearchOpts) string {
	var parts []string
	if opts.ProjectName != "" {
		parts = append(parts, fmt.Sprintf("project_name = %q", opts.ProjectName))
	}
	if opts.SessionID != "" {
		parts = append(parts, fmt.Sprintf("session_id = %q", opts.SessionID))
	}
	if !opts.DateRange.IsZero {
		parts = append(parts, fmt.Sprintf("timestamp_unix >= %d", opts.DateRange.StartUnix))
		parts = append(parts, fmt.Sprintf("timestamp_unix < %d", opts.DateRange.EndUnix))
	}
	return strings.Join(parts, " AND ")
}

func buildEventFilter(opts EventSearchOpts) string {
	var parts []string
	if opts.ProjectName != "" {
		parts = append(parts, fmt.Sprintf("project_name = %q", opts.ProjectName))
	}
	if opts.SessionID != "" {
		parts = append(parts, fmt.Sprintf("session_id = %q", opts.SessionID))
	}
	if opts.HookType != "" {
		parts = append(parts, fmt.Sprintf("hook_type = %q", opts.HookType))
	}
	if opts.ToolName != "" {
		parts = append(parts, fmt.Sprintf("tool_name = %q", opts.ToolName))
	}
	if !opts.DateRange.IsZero {
		parts = append(parts, fmt.Sprintf("timestamp_unix >= %d", opts.DateRange.StartUnix))
		parts = append(parts, fmt.Sprintf("timestamp_unix < %d", opts.DateRange.EndUnix))
	}
	return strings.Join(parts, " AND ")
}

// BuildEventFilter is exported for tools that need custom filter construction.
func BuildEventFilter(opts EventSearchOpts) string {
	return buildEventFilter(opts)
}

// SemanticSearchPrompts is a stub — MeiliSearch has no vector search.
func (c *MeiliClient) SemanticSearchPrompts(ctx context.Context, queryText string, opts PromptSearchOpts) ([]PromptHit, error) {
	return nil, nil
}

// SemanticSearchEvents is a stub — MeiliSearch has no vector search.
func (c *MeiliClient) SemanticSearchEvents(ctx context.Context, queryText string, opts EventSearchOpts) ([]EventHit, error) {
	return nil, nil
}

// HybridSearch is a stub — MeiliSearch has no vector search.
func (c *MeiliClient) HybridSearch(ctx context.Context, queryText string, opts EventSearchOpts) ([]EventHit, error) {
	return nil, nil
}

func clamp(val, defaultVal, maxVal int64) int64 {
	if val <= 0 {
		return defaultVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}
