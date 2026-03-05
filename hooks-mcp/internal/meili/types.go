// Package meili defines the Searcher interface and hit types used by tool handlers.
// The MilvusClient in the milvus package implements this interface.
// The original MeiliClient is preserved in meiliclient.go (build-tagged ignore).
package meili

import (
	"context"
	"fmt"
	"strings"

	"hooks-mcp/internal/dateparse"
)

// Searcher is the interface tools use to query the backend.
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
	FullTextSearchEvents(ctx context.Context, query string, opts EventSearchOpts) ([]EventHit, error)
	FullTextSearchPrompts(ctx context.Context, query string, opts PromptSearchOpts) ([]PromptHit, error)
}

// SessionSearchOpts controls session queries.
type SessionSearchOpts struct {
	Query       string
	SessionID   string
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
	Approximate  bool                      // true if results were capped (e.g. 10k limit)
	Distribution map[string]map[string]int // facet name -> value -> count
}

// BuildEventFilter builds a Milvus-syntax filter string from EventSearchOpts.
func BuildEventFilter(opts EventSearchOpts) string {
	var parts []string
	if opts.ProjectName != "" {
		parts = append(parts, fmt.Sprintf("project_name == %q", opts.ProjectName))
	}
	if opts.SessionID != "" {
		parts = append(parts, fmt.Sprintf("session_id == %q", opts.SessionID))
	}
	if opts.HookType != "" {
		parts = append(parts, fmt.Sprintf("hook_type == %q", opts.HookType))
	}
	if opts.ToolName != "" {
		parts = append(parts, fmt.Sprintf("tool_name == %q", opts.ToolName))
	}
	if !opts.DateRange.IsZero {
		parts = append(parts, fmt.Sprintf("timestamp_unix >= %d", opts.DateRange.StartUnix))
		parts = append(parts, fmt.Sprintf("timestamp_unix < %d", opts.DateRange.EndUnix))
	}
	return strings.Join(parts, " && ")
}
