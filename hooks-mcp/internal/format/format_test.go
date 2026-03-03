package format

import (
	"strings"
	"testing"
)

func TestTable_Basic(t *testing.T) {
	t.Parallel()
	out := Table(
		[]string{"ID", "Name", "Count"},
		[][]string{
			{"abc", "alpha", "10"},
			{"defgh", "beta", "200"},
		},
	)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 { // header + separator + 2 data rows
		t.Fatalf("expected 4 lines, got %d: %q", len(lines), out)
	}
	if !strings.Contains(lines[0], "ID") || !strings.Contains(lines[0], "Name") {
		t.Errorf("header missing columns: %q", lines[0])
	}
	if !strings.Contains(lines[1], "─") {
		t.Errorf("expected separator line: %q", lines[1])
	}
}

func TestTable_Empty(t *testing.T) {
	t.Parallel()
	out := Table([]string{}, nil)
	if out != "" {
		t.Errorf("expected empty string, got %q", out)
	}
}

func TestTree_Basic(t *testing.T) {
	t.Parallel()
	out := Tree([]TreeNode{
		{
			Label: "root",
			Children: []TreeNode{
				{Label: "child1"},
				{Label: "child2", Children: []TreeNode{
					{Label: "grandchild"},
				}},
			},
		},
	})
	if !strings.Contains(out, "root") {
		t.Errorf("missing root: %q", out)
	}
	if !strings.Contains(out, "child1") {
		t.Errorf("missing child1: %q", out)
	}
	if !strings.Contains(out, "grandchild") {
		t.Errorf("missing grandchild: %q", out)
	}
}

func TestBarChart_Basic(t *testing.T) {
	t.Parallel()
	out := BarChart([]BarItem{
		{Label: "Read", Value: 100},
		{Label: "Edit", Value: 50},
		{Label: "Bash", Value: 25},
	}, 20)
	if !strings.Contains(out, "Read") {
		t.Errorf("missing Read: %q", out)
	}
	if !strings.Contains(out, "█") {
		t.Errorf("missing bar characters: %q", out)
	}
}

func TestBarChart_Empty(t *testing.T) {
	t.Parallel()
	out := BarChart(nil, 20)
	if out != "" {
		t.Errorf("expected empty, got %q", out)
	}
}

func TestBarChart_AllZero(t *testing.T) {
	t.Parallel()
	out := BarChart([]BarItem{
		{Label: "A", Value: 0},
		{Label: "B", Value: 0},
	}, 20)
	if !strings.Contains(out, "A") || !strings.Contains(out, "B") {
		t.Errorf("expected labels in output: %q", out)
	}
}

func TestBarChart_Single(t *testing.T) {
	t.Parallel()
	out := BarChart([]BarItem{
		{Label: "Only", Value: 42},
	}, 20)
	if !strings.Contains(out, "Only") || !strings.Contains(out, "█") {
		t.Errorf("expected label and bar: %q", out)
	}
}

func TestFormatDuration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		seconds float64
		want    string
	}{
		{0, "0s"},
		{45, "45s"},
		{60, "1m0s"},
		{90, "1m30s"},
		{3600, "1h0m"},
		{3661, "1h1m"},
	}
	for _, tt := range tests {
		if got := FormatDuration(tt.seconds); got != tt.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.seconds, got, tt.want)
		}
	}
}

func TestFormatCost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		usd  float64
		want string
	}{
		{0, "$0.00"},
		{0.005, "$0.0050"},
		{0.01, "$0.01"},
		{1.50, "$1.50"},
	}
	for _, tt := range tests {
		if got := FormatCost(tt.usd); got != tt.want {
			t.Errorf("FormatCost(%v) = %q, want %q", tt.usd, got, tt.want)
		}
	}
}

func TestFormatTokens(t *testing.T) {
	t.Parallel()
	tests := []struct {
		tokens int64
		want   string
	}{
		{500, "500"},
		{1500, "1.5K"},
		{1_500_000, "1.5M"},
	}
	for _, tt := range tests {
		if got := FormatTokens(tt.tokens); got != tt.want {
			t.Errorf("FormatTokens(%v) = %q, want %q", tt.tokens, got, tt.want)
		}
	}
}

func TestShortID(t *testing.T) {
	t.Parallel()
	if got := ShortID("af7deb64-1234-5678-abcd-ef0123456789"); got != "af7deb64" {
		t.Errorf("ShortID = %q", got)
	}
	if got := ShortID("short"); got != "short" {
		t.Errorf("ShortID = %q", got)
	}
}

func TestFormatTimestamp(t *testing.T) {
	t.Parallel()
	if got := FormatTimestamp(0); got != "-" {
		t.Errorf("FormatTimestamp(0) = %q", got)
	}
	got := FormatTimestamp(1772323200) // 2026-03-01 00:00:00 UTC
	if !strings.Contains(got, "2026-03-01") {
		t.Errorf("FormatTimestamp = %q", got)
	}
}

func TestFormatTimestampISO(t *testing.T) {
	t.Parallel()
	got := FormatTimestampISO("2026-02-28T15:30:00Z")
	if got != "2026-02-28 15:30" {
		t.Errorf("FormatTimestampISO = %q", got)
	}
}

func TestTruncatePrompt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{"short", "short", 10, "short"},
		{"long", "this is a long prompt", 10, "this is a…"},
		{"zero", "hello", 0, ""},
		{"negative", "hello", -1, ""},
		{"one", "hello", 1, "…"},
		{"multibyte", "你好世界", 3, "你好…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TruncatePrompt(tt.s, tt.maxLen); got != tt.want {
				t.Errorf("TruncatePrompt(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}
