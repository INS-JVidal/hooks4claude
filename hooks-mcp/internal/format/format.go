// Package format provides pure formatting functions for MCP tool output.
// All functions are side-effect-free and have no external dependencies.
package format

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Table renders rows as an aligned plain-text table.
// headers defines column names; rows is a slice of string slices.
func Table(headers []string, rows [][]string) string {
	if len(headers) == 0 {
		return ""
	}

	// Compute column widths (rune-aware for non-ASCII text).
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, row := range rows {
		for i := 0; i < len(widths) && i < len(row); i++ {
			if w := utf8.RuneCountInString(row[i]); w > widths[i] {
				widths[i] = w
			}
		}
	}

	var b strings.Builder

	// Header row.
	for i, h := range headers {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(padRight(h, widths[i]))
	}
	b.WriteByte('\n')

	// Separator.
	for i, w := range widths {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(strings.Repeat("─", w))
	}
	b.WriteByte('\n')

	// Data rows.
	for _, row := range rows {
		for i := 0; i < len(widths); i++ {
			if i > 0 {
				b.WriteString("  ")
			}
			val := ""
			if i < len(row) {
				val = row[i]
			}
			b.WriteString(padRight(val, widths[i]))
		}
		b.WriteByte('\n')
	}

	return b.String()
}

// Tree renders a hierarchical tree structure as an ASCII tree.
// Each node is a label with optional children.
type TreeNode struct {
	Label    string
	Children []TreeNode
}

// Tree renders a slice of root nodes as an ASCII tree.
func Tree(roots []TreeNode) string {
	var b strings.Builder
	for i, root := range roots {
		renderTree(&b, root, "", i == len(roots)-1)
	}
	return b.String()
}

func renderTree(b *strings.Builder, node TreeNode, prefix string, isLast bool) {
	connector := "├── "
	if isLast {
		connector = "└── "
	}
	if prefix == "" {
		// Root node — no connector.
		b.WriteString(node.Label)
	} else {
		b.WriteString(prefix + connector + node.Label)
	}
	b.WriteByte('\n')

	childPrefix := prefix
	if prefix != "" {
		if isLast {
			childPrefix += "    "
		} else {
			childPrefix += "│   "
		}
	}

	for i, child := range node.Children {
		renderTree(b, child, childPrefix, i == len(node.Children)-1)
	}
}

// BarChart renders a simple horizontal bar chart.
// items is a slice of (label, value) pairs. maxWidth is the max bar length in characters.
func BarChart(items []BarItem, maxWidth int) string {
	if len(items) == 0 {
		return ""
	}

	// Find max value and max label width.
	var maxVal int
	maxLabel := 0
	for _, item := range items {
		if item.Value > maxVal {
			maxVal = item.Value
		}
		if len(item.Label) > maxLabel {
			maxLabel = len(item.Label)
		}
	}
	if maxVal == 0 {
		maxVal = 1
	}

	var b strings.Builder
	for _, item := range items {
		barLen := item.Value * maxWidth / maxVal
		if barLen < 0 {
			barLen = 0
		}
		if barLen == 0 && item.Value > 0 {
			barLen = 1
		}
		b.WriteString(fmt.Sprintf("%-*s  %s %d\n",
			maxLabel, item.Label,
			strings.Repeat("█", barLen),
			item.Value))
	}
	return b.String()
}

// BarItem is a single entry in a bar chart.
type BarItem struct {
	Label string
	Value int
}

// FormatDuration formats seconds as a human-readable duration.
func FormatDuration(seconds float64) string {
	if seconds < 60 {
		return fmt.Sprintf("%.0fs", seconds)
	}
	if seconds < 3600 {
		m := int(seconds) / 60
		s := int(seconds) % 60
		return fmt.Sprintf("%dm%ds", m, s)
	}
	h := int(seconds) / 3600
	m := (int(seconds) % 3600) / 60
	return fmt.Sprintf("%dh%dm", h, m)
}

// FormatCost formats USD cost with appropriate precision.
func FormatCost(usd float64) string {
	if usd == 0 {
		return "$0.00"
	}
	if usd < 0.01 {
		return fmt.Sprintf("$%.4f", usd)
	}
	return fmt.Sprintf("$%.2f", usd)
}

// FormatTokens formats a token count with K/M suffixes.
func FormatTokens(tokens int64) string {
	if tokens < 1000 {
		return fmt.Sprintf("%d", tokens)
	}
	if tokens < 1_000_000 {
		return fmt.Sprintf("%.1fK", float64(tokens)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(tokens)/1_000_000)
}

// ShortID returns the first 8 characters of a UUID.
func ShortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// FormatTimestamp formats a unix timestamp as a readable string.
func FormatTimestamp(unix int64) string {
	if unix == 0 {
		return "-"
	}
	return time.Unix(unix, 0).UTC().Format("2006-01-02 15:04")
}

// FormatTimestampISO formats an ISO 8601 string to a shorter display format.
func FormatTimestampISO(iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		// Try with milliseconds.
		t, err = time.Parse("2006-01-02T15:04:05.000Z", iso)
		if err != nil {
			return iso
		}
	}
	return t.UTC().Format("2006-01-02 15:04")
}

// TruncatePrompt truncates a prompt string to maxLen runes with an ellipsis.
func TruncatePrompt(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen == 1 {
		return "…"
	}
	return string(runes[:maxLen-1]) + "…"
}

func padRight(s string, width int) string {
	w := utf8.RuneCountInString(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}
