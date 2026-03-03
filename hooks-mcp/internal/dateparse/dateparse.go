// Package dateparse converts human-friendly date range strings into
// unix timestamps and ISO 8601 strings for MeiliSearch queries.
//
// Supports: "", "today", "yesterday", "last N days", "last N hours",
// "2026-02-28", "2026-02-28..2026-03-01". All times are UTC.
package dateparse

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DateRange holds both unix timestamps and ISO strings for a date range.
// Events/prompts indexes filter on timestamp_unix (int64), while the
// sessions index filters on started_at (ISO 8601 string).
type DateRange struct {
	StartUnix int64
	EndUnix   int64
	StartISO  string // ISO 8601 (e.g. "2026-02-28T00:00:00Z")
	EndISO    string
	IsZero    bool // true when input was empty (no filter)
}

var (
	reLastNDays  = regexp.MustCompile(`(?i)^last\s+(\d+)\s+days?$`)
	reLastNHours = regexp.MustCompile(`(?i)^last\s+(\d+)\s+hours?$`)
	reDateRange  = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\.\.(\d{4}-\d{2}-\d{2})$`)
	reSingleDate = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})$`)
)

// ParseRange parses a human-friendly date range string into a DateRange.
// Uses the provided now time for relative calculations.
func ParseRange(input string, now time.Time) (DateRange, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return DateRange{IsZero: true}, nil
	}

	now = now.UTC()

	switch strings.ToLower(input) {
	case "today":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		end := start.Add(24 * time.Hour)
		return makeRange(start, end), nil

	case "yesterday":
		end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		start := end.Add(-24 * time.Hour)
		return makeRange(start, end), nil
	}

	if m := reLastNDays.FindStringSubmatch(input); m != nil {
		n, _ := strconv.Atoi(m[1])
		if n < 1 {
			return DateRange{}, fmt.Errorf("last N days requires N >= 1, got %d", n)
		}
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Add(-time.Duration(n-1) * 24 * time.Hour)
		end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Add(24 * time.Hour)
		return makeRange(start, end), nil
	}

	if m := reLastNHours.FindStringSubmatch(input); m != nil {
		n, _ := strconv.Atoi(m[1])
		if n < 1 {
			return DateRange{}, fmt.Errorf("last N hours requires N >= 1, got %d", n)
		}
		end := now
		start := end.Add(-time.Duration(n) * time.Hour)
		return makeRange(start, end), nil
	}

	if m := reDateRange.FindStringSubmatch(input); m != nil {
		start, err := time.Parse("2006-01-02", m[1])
		if err != nil {
			return DateRange{}, fmt.Errorf("invalid start date %q: %w", m[1], err)
		}
		end, err := time.Parse("2006-01-02", m[2])
		if err != nil {
			return DateRange{}, fmt.Errorf("invalid end date %q: %w", m[2], err)
		}
		// End is exclusive — add one day.
		end = end.Add(24 * time.Hour)
		return makeRange(start.UTC(), end.UTC()), nil
	}

	if m := reSingleDate.FindStringSubmatch(input); m != nil {
		start, err := time.Parse("2006-01-02", m[1])
		if err != nil {
			return DateRange{}, fmt.Errorf("invalid date %q: %w", m[1], err)
		}
		end := start.Add(24 * time.Hour)
		return makeRange(start.UTC(), end.UTC()), nil
	}

	return DateRange{}, fmt.Errorf("unrecognized date range: %q (try: today, yesterday, last N days, last N hours, YYYY-MM-DD, YYYY-MM-DD..YYYY-MM-DD)", input)
}

func makeRange(start, end time.Time) DateRange {
	return DateRange{
		StartUnix: start.Unix(),
		EndUnix:   end.Unix(),
		StartISO:  start.Format(time.RFC3339),
		EndISO:    end.Format(time.RFC3339),
	}
}
