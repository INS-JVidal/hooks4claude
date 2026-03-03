# format — Pure formatting functions for MCP tool output

```go
func Table(headers []string, rows [][]string) string
func Tree(roots []TreeNode) string
func BarChart(items []BarItem, maxWidth int) string
func FormatDuration(seconds float64) string
func FormatCost(usd float64) string
func FormatTokens(tokens int64) string
func ShortID(id string) string                  // first 8 chars
func FormatTimestamp(unix int64) string          // "2006-01-02 15:04"
func FormatTimestampISO(iso string) string       // ISO → "2006-01-02 15:04"
func TruncatePrompt(s string, maxLen int) string // rune-safe truncation with …
```

Types: `TreeNode{Label, Children}`, `BarItem{Label, Value}`.

All pure functions, no external dependencies. No side effects.
