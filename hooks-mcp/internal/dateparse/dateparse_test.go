package dateparse

import (
	"testing"
	"time"
)

// Fixed reference time: 2026-03-02 15:30:00 UTC
var refTime = time.Date(2026, 3, 2, 15, 30, 0, 0, time.UTC)

func TestParseRange_Empty(t *testing.T) {
	t.Parallel()
	dr, err := ParseRange("", refTime)
	if err != nil {
		t.Fatal(err)
	}
	if !dr.IsZero {
		t.Error("expected IsZero=true for empty input")
	}
}

func TestParseRange_Today(t *testing.T) {
	t.Parallel()
	dr, err := ParseRange("today", refTime)
	if err != nil {
		t.Fatal(err)
	}
	// 2026-03-02 00:00:00 UTC
	wantStart := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC).Unix()
	wantEnd := time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC).Unix()
	if dr.StartUnix != wantStart {
		t.Errorf("StartUnix = %d, want %d", dr.StartUnix, wantStart)
	}
	if dr.EndUnix != wantEnd {
		t.Errorf("EndUnix = %d, want %d", dr.EndUnix, wantEnd)
	}
	if dr.StartISO != "2026-03-02T00:00:00Z" {
		t.Errorf("StartISO = %q", dr.StartISO)
	}
}

func TestParseRange_Yesterday(t *testing.T) {
	t.Parallel()
	dr, err := ParseRange("yesterday", refTime)
	if err != nil {
		t.Fatal(err)
	}
	wantStart := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC).Unix()
	wantEnd := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC).Unix()
	if dr.StartUnix != wantStart {
		t.Errorf("StartUnix = %d, want %d", dr.StartUnix, wantStart)
	}
	if dr.EndUnix != wantEnd {
		t.Errorf("EndUnix = %d, want %d", dr.EndUnix, wantEnd)
	}
}

func TestParseRange_LastNDays(t *testing.T) {
	t.Parallel()
	dr, err := ParseRange("last 3 days", refTime)
	if err != nil {
		t.Fatal(err)
	}
	// "last 3 days" from 2026-03-02: start = 2026-02-28 00:00, end = 2026-03-03 00:00
	wantStart := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC).Unix()
	wantEnd := time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC).Unix()
	if dr.StartUnix != wantStart {
		t.Errorf("StartUnix = %d, want %d", dr.StartUnix, wantStart)
	}
	if dr.EndUnix != wantEnd {
		t.Errorf("EndUnix = %d, want %d", dr.EndUnix, wantEnd)
	}
}

func TestParseRange_Last1Day(t *testing.T) {
	t.Parallel()
	dr, err := ParseRange("last 1 day", refTime)
	if err != nil {
		t.Fatal(err)
	}
	// "last 1 day" = today
	wantStart := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC).Unix()
	if dr.StartUnix != wantStart {
		t.Errorf("StartUnix = %d, want %d", dr.StartUnix, wantStart)
	}
}

func TestParseRange_LastNHours(t *testing.T) {
	t.Parallel()
	dr, err := ParseRange("last 6 hours", refTime)
	if err != nil {
		t.Fatal(err)
	}
	wantStart := refTime.Add(-6 * time.Hour).Unix()
	wantEnd := refTime.Unix()
	if dr.StartUnix != wantStart {
		t.Errorf("StartUnix = %d, want %d", dr.StartUnix, wantStart)
	}
	if dr.EndUnix != wantEnd {
		t.Errorf("EndUnix = %d, want %d", dr.EndUnix, wantEnd)
	}
}

func TestParseRange_SingleDate(t *testing.T) {
	t.Parallel()
	dr, err := ParseRange("2026-02-28", refTime)
	if err != nil {
		t.Fatal(err)
	}
	wantStart := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC).Unix()
	wantEnd := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC).Unix()
	if dr.StartUnix != wantStart {
		t.Errorf("StartUnix = %d, want %d", dr.StartUnix, wantStart)
	}
	if dr.EndUnix != wantEnd {
		t.Errorf("EndUnix = %d, want %d", dr.EndUnix, wantEnd)
	}
}

func TestParseRange_DateRange(t *testing.T) {
	t.Parallel()
	dr, err := ParseRange("2026-02-28..2026-03-01", refTime)
	if err != nil {
		t.Fatal(err)
	}
	wantStart := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC).Unix()
	// End date is exclusive: 2026-03-01 + 1 day = 2026-03-02
	wantEnd := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC).Unix()
	if dr.StartUnix != wantStart {
		t.Errorf("StartUnix = %d, want %d", dr.StartUnix, wantStart)
	}
	if dr.EndUnix != wantEnd {
		t.Errorf("EndUnix = %d, want %d", dr.EndUnix, wantEnd)
	}
}

func TestParseRange_CaseInsensitive(t *testing.T) {
	t.Parallel()
	dr, err := ParseRange("Today", refTime)
	if err != nil {
		t.Fatal(err)
	}
	if dr.IsZero {
		t.Error("expected non-zero range for 'Today'")
	}
}

func TestParseRange_WhitespaceTrimmed(t *testing.T) {
	t.Parallel()
	dr, err := ParseRange("  today  ", refTime)
	if err != nil {
		t.Fatal(err)
	}
	if dr.IsZero {
		t.Error("expected non-zero range")
	}
}

func TestParseRange_Invalid(t *testing.T) {
	t.Parallel()
	_, err := ParseRange("not a date", refTime)
	if err == nil {
		t.Error("expected error for invalid input")
	}
}

func TestParseRange_Last0Days(t *testing.T) {
	t.Parallel()
	_, err := ParseRange("last 0 days", refTime)
	if err == nil {
		t.Error("expected error for last 0 days")
	}
}

func TestParseRange_Last0Hours(t *testing.T) {
	t.Parallel()
	_, err := ParseRange("last 0 hours", refTime)
	if err == nil {
		t.Error("expected error for last 0 hours")
	}
}

func TestParseRange_LastNDaysCaseVariation(t *testing.T) {
	t.Parallel()
	dr1, err := ParseRange("Last 3 Days", refTime)
	if err != nil {
		t.Fatal(err)
	}
	dr2, err := ParseRange("last 3 days", refTime)
	if err != nil {
		t.Fatal(err)
	}
	if dr1.StartUnix != dr2.StartUnix || dr1.EndUnix != dr2.EndUnix {
		t.Errorf("case variation should produce same range: %v vs %v", dr1, dr2)
	}
}

func TestParseRange_ISOFormat(t *testing.T) {
	t.Parallel()
	dr, err := ParseRange("2026-02-28", refTime)
	if err != nil {
		t.Fatal(err)
	}
	if dr.StartISO != "2026-02-28T00:00:00Z" {
		t.Errorf("StartISO = %q", dr.StartISO)
	}
	if dr.EndISO != "2026-03-01T00:00:00Z" {
		t.Errorf("EndISO = %q", dr.EndISO)
	}
}
