# hook-shim — Rust shim for Claude Code hook events

Tiny Rust binary (~80 lines) spawned per hook event by Claude Code. Replaces the Go hook-client binary for lower startup latency.

1. Reads stdin (1 MiB cap)
2. Connects to `/tmp/hook-client.sock` (50ms timeout, exit 0 on failure)
3. If PreToolUse Read: sends `0x02` cache query, reads `0x03` response, prints `hookSpecificOutput` to stdout
4. Otherwise: sends `0x01` fire-and-forget event
5. Always exits 0

Dependencies: `serde_json` only.

Build: `cargo build --release` → `target/release/hook-shim`
