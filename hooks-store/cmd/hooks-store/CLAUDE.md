# cmd/hooks-store — Entry point for the hooks-store binary

## main.go

CLI flags: --port (env: HOOKS_STORE_PORT, default: 9800), --milvus-url (env: MILVUS_URL, default: http://localhost:19530), --milvus-token (env: MILVUS_TOKEN), --events-col (env: MILVUS_EVENTS_COL, default: hook_events), --prompts-col (env: MILVUS_PROMPTS_COL, default: hook_prompts), --sessions-col (env: MILVUS_SESSIONS_COL, default: hook_sessions), --embed-url (env: EMBED_SVC_URL, default: http://localhost:8900), --uds-sock (env: HOOKS_STORE_SOCK), --headless.

Wiring: connects Milvus (3 collections + embed-svc) → creates ingest.Server → creates eventCh (cap 256) → wires SetOnIngest callback (non-blocking send) → starts HTTP server in goroutine → optionally starts UDS server → runs tui.Run() (blocks) → shutdown via sync.Once.

`var version = "dev"` — set by ldflags at build time.

Imports: `ingest`, `store`, `tui`.
