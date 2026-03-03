# dateparse — Human-friendly date range parsing

```go
type DateRange struct {
    StartUnix int64   // for events/prompts (timestamp_unix filter)
    EndUnix   int64
    StartISO  string  // for sessions (started_at filter, ISO 8601)
    EndISO    string
    IsZero    bool    // true when input was empty (no filter)
}

func ParseRange(input string, now time.Time) (DateRange, error)
```

Supports: `""` (zero/no filter), `"today"`, `"yesterday"`, `"last N days"`, `"last N hours"`, `"YYYY-MM-DD"`, `"YYYY-MM-DD..YYYY-MM-DD"`. All UTC. Case-insensitive. Whitespace-trimmed.

Returns both unix timestamps AND ISO strings — sessions index filters on `started_at` (string), events/prompts filter on `timestamp_unix` (int64).

No external dependencies.
