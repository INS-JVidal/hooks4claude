#!/usr/bin/env bash
# Unified startup script for hooks4claude pipeline.
# Detects already-running services, starts what's missing, validates the pipeline, launches monitor.
set -uo pipefail
# NOTE: no set -e — we handle errors per-function via || true in main.
# set -e would abort on any non-zero return (e.g. curl health check, build failure).

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LOGS="$ROOT/logs"
mkdir -p "$LOGS"

export PATH="/usr/local/go/bin:$HOME/.cargo/bin:$PATH"

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

# --- Status tracking ---
declare -A STATUS  # component -> "running" | "started" | "failed" | "skipped"
declare -A PORTS   # component -> port/socket info

# --- Helpers ---

log_info()  { printf "${CYAN}[INFO]${NC}  %s\n" "$1"; }
log_ok()    { printf "${GREEN}[OK]${NC}    %s\n" "$1"; }
log_warn()  { printf "${YELLOW}[WARN]${NC}  %s\n" "$1"; }
log_fail()  { printf "${RED}[FAIL]${NC}  %s\n" "$1"; }

# wait_healthy_seconds NAME URL [TIMEOUT_S]
# Polls a URL until it returns 200, with progress dots.
wait_healthy_seconds() {
    local name="$1" url="$2" timeout="${3:-15}"
    local deadline=$((SECONDS + timeout))
    printf "  Waiting for %s " "$name"
    while [ "$SECONDS" -lt "$deadline" ]; do
        if curl -sf --max-time 2 "$url" >/dev/null 2>&1; then
            printf " ${GREEN}ready${NC}\n"
            return 0
        fi
        printf "."
        sleep 0.5
    done
    printf " ${RED}timeout (${timeout}s)${NC}\n"
    return 1
}

# ensure_binary DIR BINARY NAME
# Builds a binary if it doesn't exist. Returns 1 on failure.
ensure_binary() {
    local dir="$1" binary="$2" name="$3"
    if [ ! -x "$ROOT/$dir/$binary" ]; then
        log_info "Building $name..."
        if ! make -C "$ROOT/$dir" build >>"$LOGS/${name}-build.log" 2>&1; then
            log_fail "Failed to build $name (see logs/${name}-build.log)"
            return 1
        fi
        # Verify the binary actually appeared after build.
        if [ ! -x "$ROOT/$dir/$binary" ]; then
            log_fail "Build succeeded but binary not found at $dir/$binary"
            return 1
        fi
    fi
    return 0
}

# --- Preflight validation ---

validate_prerequisites() {
    local errors=0

    printf "${BOLD}Preflight checks${NC}\n"
    echo "────────────────────────────────────────"

    # Required tools
    for cmd in curl make; do
        if command -v "$cmd" >/dev/null 2>&1; then
            log_ok "$cmd found"
        else
            log_fail "$cmd not found — required"
            errors=$((errors + 1))
        fi
    done

    # Docker (required for Milvus)
    if command -v docker >/dev/null 2>&1; then
        if docker info >/dev/null 2>&1; then
            log_ok "Docker running"
        else
            log_fail "Docker installed but daemon not running"
            errors=$((errors + 1))
        fi
    else
        log_fail "Docker not found — required for Milvus"
        errors=$((errors + 1))
    fi

    # Docker compose file
    if [ -f "$ROOT/docker/docker-compose.yml" ]; then
        log_ok "docker-compose.yml found"
    else
        log_fail "docker/docker-compose.yml missing"
        errors=$((errors + 1))
    fi

    # Go compiler
    if command -v go >/dev/null 2>&1; then
        log_ok "Go found ($(go version 2>/dev/null | awk '{print $3}'))"
    else
        # Check if all Go binaries are pre-built
        local missing_go_bins=0
        for bin in hooks-store/bin/hooks-store hooks-client/bin/hook-client hooks-monitor/bin/monitor hooks-mcp/bin/hooks-mcp; do
            [ ! -x "$ROOT/$bin" ] && missing_go_bins=$((missing_go_bins + 1))
        done
        if [ "$missing_go_bins" -gt 0 ]; then
            log_fail "Go not found and $missing_go_bins Go binaries not pre-built"
            errors=$((errors + 1))
        else
            log_ok "Go not found but all binaries pre-built"
        fi
    fi

    # Rust/cargo
    if command -v cargo >/dev/null 2>&1; then
        log_ok "Cargo found"
    else
        if [ -x "$ROOT/embed-svc/target/release/embed-svc" ]; then
            log_ok "Cargo not found but embed-svc pre-built"
        else
            log_warn "Cargo not found — embed-svc cannot be built (dense vectors disabled)"
        fi
    fi

    # embed-svc ONNX model
    local model_dir="$ROOT/embed-svc/models/all-MiniLM-L6-v2"
    if [ -f "$model_dir/model.onnx" ] && [ -f "$model_dir/tokenizer.json" ]; then
        log_ok "Embedding model present"
    else
        log_warn "Embedding model missing — will attempt download"
    fi

    # ONNX Runtime library
    local ort_lib="$ROOT/embed-svc/lib/libonnxruntime.so"
    if [ -f "$ort_lib" ]; then
        log_ok "ONNX Runtime library present"
    elif [ -L "$ort_lib" ] && [ ! -e "$ort_lib" ]; then
        log_warn "ONNX Runtime: broken symlink at $ort_lib"
    else
        log_warn "ONNX Runtime library missing — will attempt download on build"
    fi

    # Stale sockets
    for sock in /tmp/hook-client.sock /tmp/hooks-store.sock /tmp/hooks-store-pub.sock; do
        if [ -e "$sock" ] && ! fuser "$sock" >/dev/null 2>&1; then
            log_warn "Stale socket: $sock (removing)"
            rm -f "$sock"
        fi
    done

    # Port conflicts from non-pipeline processes
    # Milvus uses 9091 for healthz, not 19530; check each service's actual health endpoint
    for port_info in "9800:hooks-store:http://localhost:9800/health" "8900:embed-svc:http://localhost:8900/health" "8080:hooks-monitor:http://localhost:8080/health"; do
        local port="${port_info%%:*}"
        local rest="${port_info#*:}"
        local svc="${rest%%:*}"
        local health_url="${rest#*:}"
        if ss -tlnp 2>/dev/null | grep -q ":${port} "; then
            if ! curl -sf --max-time 2 "$health_url" >/dev/null 2>&1; then
                log_warn "Port $port in use by unknown process (expected by $svc)"
            fi
        fi
    done
    # Milvus: check gRPC port 19530 via the management healthz on 9091
    if ss -tlnp 2>/dev/null | grep -q ":19530 "; then
        if ! curl -sf --max-time 2 "http://localhost:9091/healthz" >/dev/null 2>&1; then
            log_warn "Port 19530 in use by unknown process (expected by Milvus)"
        fi
    fi

    echo ""

    if [ "$errors" -gt 0 ]; then
        log_fail "$errors critical prerequisite(s) missing — aborting"
        echo ""
        exit 1
    fi
}

# --- 1. Milvus (Docker) ---

check_or_start_milvus() {
    local name="Milvus"
    PORTS[$name]=":19530"

    if curl -sf --max-time 3 http://localhost:9091/healthz >/dev/null 2>&1; then
        STATUS[$name]="running"
        log_ok "$name already running"
        return 0
    fi

    log_info "Starting $name via Docker Compose..."
    if ! docker compose -f "$ROOT/docker/docker-compose.yml" up -d >>"$LOGS/docker-compose.log" 2>&1; then
        log_fail "$name failed to start (see logs/docker-compose.log)"
        STATUS[$name]="failed"
        return 1
    fi

    if wait_healthy_seconds "$name" "http://localhost:9091/healthz" 60; then
        STATUS[$name]="started"
        log_ok "$name started"
    else
        log_fail "$name did not become healthy within 60s (see logs/docker-compose.log)"
        STATUS[$name]="failed"
        return 1
    fi
}

# --- 2. embed-svc ---

check_or_start_embed_svc() {
    local name="embed-svc"
    PORTS[$name]=":8900"

    if curl -sf --max-time 2 http://localhost:8900/health >/dev/null 2>&1; then
        STATUS[$name]="running"
        log_ok "$name already running"
        return 0
    fi

    # Check model files exist — download if missing.
    local model_dir="$ROOT/embed-svc/models/all-MiniLM-L6-v2"
    if [ ! -f "$model_dir/model.onnx" ] || [ ! -f "$model_dir/tokenizer.json" ]; then
        log_info "Downloading embedding model..."
        if ! make -C "$ROOT/embed-svc" download-model >>"$LOGS/embed-model-download.log" 2>&1; then
            log_warn "Model download failed — $name skipped (see logs/embed-model-download.log)"
            STATUS[$name]="skipped"
            return 0
        fi
        # Verify download actually produced the files.
        if [ ! -f "$model_dir/model.onnx" ]; then
            log_warn "Model download completed but model.onnx not found — $name skipped"
            STATUS[$name]="skipped"
            return 0
        fi
    fi

    # Check ONNX Runtime library — download via make if missing.
    local ort_lib="$ROOT/embed-svc/lib/libonnxruntime.so"
    if [ ! -f "$ort_lib" ]; then
        log_info "Downloading ONNX Runtime library..."
        if ! make -C "$ROOT/embed-svc" download-onnxruntime >>"$LOGS/embed-ort-download.log" 2>&1; then
            log_warn "ONNX Runtime download failed — $name skipped (see logs/embed-ort-download.log)"
            STATUS[$name]="skipped"
            return 0
        fi
        if [ ! -f "$ort_lib" ]; then
            log_warn "ONNX Runtime download completed but library not found — $name skipped"
            STATUS[$name]="skipped"
            return 0
        fi
    fi

    # Build if needed.
    local binary="$ROOT/embed-svc/target/release/embed-svc"
    if [ ! -x "$binary" ]; then
        log_info "Building $name (Rust, may take a minute)..."
        if ! make -C "$ROOT/embed-svc" build >>"$LOGS/embed-build.log" 2>&1; then
            log_warn "Build failed — $name skipped (see logs/embed-build.log)"
            STATUS[$name]="skipped"
            return 0
        fi
        if [ ! -x "$binary" ]; then
            log_warn "Build succeeded but binary not found — $name skipped"
            STATUS[$name]="skipped"
            return 0
        fi
    fi

    log_info "Starting $name..."
    (cd "$ROOT/embed-svc" && ORT_DYLIB_PATH="$ort_lib" "$binary" >>"$LOGS/embed-svc.log" 2>&1) &
    local pid=$!

    if wait_healthy_seconds "$name" "http://localhost:8900/health" 30; then
        STATUS[$name]="started"
        log_ok "$name started (PID $pid)"
    else
        # Check if process died immediately (crash on startup).
        if ! kill -0 "$pid" 2>/dev/null; then
            log_warn "$name crashed on startup — check logs/embed-svc.log"
            tail -5 "$LOGS/embed-svc.log" 2>/dev/null | while IFS= read -r line; do
                printf "         ${RED}%s${NC}\n" "$line"
            done
        else
            log_warn "$name running but not responding on :8900"
            kill "$pid" 2>/dev/null || true
        fi
        STATUS[$name]="skipped"
    fi
}

# --- 3. hooks-store ---

check_or_start_hooks_store() {
    local name="hooks-store"
    PORTS[$name]=":9800"

    if curl -sf --max-time 2 http://localhost:9800/health >/dev/null 2>&1; then
        STATUS[$name]="running"
        log_ok "$name already running"
        return 0
    fi

    if ! ensure_binary "hooks-store" "bin/hooks-store" "$name"; then
        STATUS[$name]="failed"
        return 1
    fi

    log_info "Starting $name..."
    HOOKS_STORE_SOCK=/tmp/hooks-store.sock \
    HOOKS_STORE_PUB_SOCK=/tmp/hooks-store-pub.sock \
        "$ROOT/hooks-store/bin/hooks-store" --headless \
        >>"$LOGS/hooks-store.log" 2>&1 &
    local pid=$!

    if wait_healthy_seconds "$name" "http://localhost:9800/health" 15; then
        STATUS[$name]="started"
        log_ok "$name started (PID $pid)"
    else
        if ! kill -0 "$pid" 2>/dev/null; then
            log_fail "$name crashed on startup — check logs/hooks-store.log"
            tail -5 "$LOGS/hooks-store.log" 2>/dev/null | while IFS= read -r line; do
                printf "         ${RED}%s${NC}\n" "$line"
            done
        else
            log_fail "$name running but not responding on :9800"
            kill "$pid" 2>/dev/null || true
        fi
        STATUS[$name]="failed"
        return 1
    fi
}

# --- 4. hook-client daemon ---

check_or_start_hook_client() {
    local name="hook-client"
    local sock="/tmp/hook-client.sock"
    PORTS[$name]="$sock"

    if [ -S "$sock" ]; then
        STATUS[$name]="running"
        log_ok "$name daemon already running"
        return 0
    fi

    if ! ensure_binary "hooks-client" "bin/hook-client" "$name"; then
        STATUS[$name]="failed"
        return 1
    fi

    log_info "Starting $name daemon..."
    HOOKS_STORE_SOCK=/tmp/hooks-store.sock \
        "$ROOT/hooks-client/bin/hook-client" daemon \
        >>"$LOGS/hook-client.log" 2>&1 &
    local pid=$!

    # Wait for socket to appear.
    local deadline=$((SECONDS + 5))
    while [ "$SECONDS" -lt "$deadline" ]; do
        if [ -S "$sock" ]; then
            STATUS[$name]="started"
            log_ok "$name daemon started (PID $pid)"
            return 0
        fi
        # Check if process already died.
        if ! kill -0 "$pid" 2>/dev/null; then
            log_fail "$name daemon crashed on startup — check logs/hook-client.log"
            tail -3 "$LOGS/hook-client.log" 2>/dev/null | while IFS= read -r line; do
                printf "         ${RED}%s${NC}\n" "$line"
            done
            STATUS[$name]="failed"
            return 1
        fi
        sleep 0.2
    done

    log_fail "$name daemon did not create socket within 5s"
    kill "$pid" 2>/dev/null || true
    STATUS[$name]="failed"
    return 1
}

# --- 4b. Ensure hook-shim binary installed ---

ensure_hook_shim() {
    local name="hook-shim"
    local src="$ROOT/hook-shim/target/release/hook-shim"
    local dest="$HOME/.local/bin/hook-shim"

    if [ -x "$dest" ]; then
        # Reinstall if source is newer than installed copy
        if [ -x "$src" ] && [ "$src" -nt "$dest" ]; then
            log_info "$name binary outdated — reinstalling..."
            cp "$src" "$dest"
            log_ok "$name updated at ~/.local/bin/hook-shim"
        else
            log_ok "$name installed at ~/.local/bin/hook-shim"
        fi
        STATUS[$name]="installed"
        return 0
    fi

    # Build if not built
    if [ ! -x "$src" ]; then
        if ! command -v cargo >/dev/null 2>&1; then
            log_warn "cargo not found — cannot build $name"
            STATUS[$name]="skipped"
            return 0
        fi
        log_info "Building $name (cargo release)..."
        if ! cargo build --release --manifest-path "$ROOT/hook-shim/Cargo.toml" >>"$LOGS/hook-shim-build.log" 2>&1; then
            log_fail "Failed to build $name (see logs/hook-shim-build.log)"
            STATUS[$name]="failed"
            return 1
        fi
    fi

    # Install
    mkdir -p "$HOME/.local/bin"
    cp "$src" "$dest"
    log_ok "$name installed at ~/.local/bin/hook-shim"
    STATUS[$name]="installed"
}

# --- 5. Ensure hooks registered ---

ensure_hooks_registered() {
    if [ ! -x "$ROOT/hooks-client/bin/hook-client" ]; then
        log_warn "hook-client not built — skipping hook registration"
        return 0
    fi

    log_info "Ensuring hooks are registered in Claude Code..."
    local output
    output=$("$ROOT/hooks-client/bin/hook-client" install-hooks --shim 2>&1) || true
    if [ -n "$output" ]; then
        log_ok "$output"
    else
        log_ok "Hooks registered"
    fi
}

# --- 6. Check hooks-mcp ---

check_hooks_mcp() {
    local name="hooks-mcp"

    if [ -x "$HOME/.local/bin/hooks-mcp" ]; then
        STATUS[$name]="installed"
        log_ok "$name installed at ~/.local/bin/hooks-mcp"
    else
        if ensure_binary "hooks-mcp" "bin/hooks-mcp" "$name"; then
            log_info "Installing $name to ~/.local/bin/..."
            if make -C "$ROOT/hooks-mcp" install >>"$LOGS/hooks-mcp-install.log" 2>&1; then
                STATUS[$name]="installed"
                log_ok "$name installed"
            else
                log_warn "$name install failed (see logs/hooks-mcp-install.log)"
                STATUS[$name]="skipped"
            fi
        else
            STATUS[$name]="skipped"
        fi
    fi

    # Check MCP registration and auto-register if missing (only if binary is installed)
    if [ "${STATUS[$name]}" = "installed" ]; then
        local settings="$HOME/.claude.json"
        if ! command -v python3 >/dev/null 2>&1; then
            log_warn "python3 not found — cannot check MCP registration"
        elif [ -f "$settings" ] && python3 -c "
import json,sys
d=json.load(open(sys.argv[1]))
sys.exit(0 if 'hooks-mcp' in d.get('mcpServers',{}) else 1)
" "$settings" 2>/dev/null; then
            log_ok "$name registered as MCP server"
        else
            log_info "Registering $name as MCP server..."
            if command -v claude >/dev/null 2>&1; then
                claude mcp add --transport stdio --scope user hooks-mcp -- \
                    "$HOME/.local/bin/hooks-mcp" >>"$LOGS/hooks-mcp-install.log" 2>&1 \
                    && log_ok "$name registered as MCP server" \
                    || log_warn "$name MCP registration failed (see logs/hooks-mcp-install.log)"
            else
                log_warn "claude CLI not found — register manually: claude mcp add --transport stdio --scope user hooks-mcp -- $HOME/.local/bin/hooks-mcp"
            fi
        fi
    fi
}

# --- Post-start validation ---

validate_pipeline() {
    local warnings=0

    echo ""
    printf "${BOLD}Pipeline health check${NC}\n"
    echo "────────────────────────────────────────"

    # 1. Milvus responds to REST API (not just healthz)
    if [ "${STATUS[Milvus]:-}" = "running" ] || [ "${STATUS[Milvus]:-}" = "started" ]; then
        local milvus_resp
        milvus_resp=$(curl -sf --max-time 3 -X POST -H 'Content-Type: application/json' -d '{}' http://localhost:19530/v2/vectordb/collections/list 2>/dev/null) || milvus_resp=""
        if echo "$milvus_resp" | grep -q '"code":0'; then
            local collections
            collections=$(echo "$milvus_resp" | python3 -c "import sys,json; print(', '.join(json.load(sys.stdin).get('data',[])))" 2>/dev/null) || collections="(parse error)"
            log_ok "Milvus REST API responding — collections: ${collections:-none}"
        else
            log_warn "Milvus healthz OK but REST API not responding on :19530"
            warnings=$((warnings + 1))
        fi
    fi

    # 2. embed-svc returns correct dimensions
    if [ "${STATUS[embed-svc]:-}" = "running" ] || [ "${STATUS[embed-svc]:-}" = "started" ]; then
        local health_json
        health_json=$(curl -sf --max-time 3 http://localhost:8900/health 2>/dev/null) || health_json=""
        if echo "$health_json" | grep -q '"dimensions":384'; then
            log_ok "embed-svc healthy — model: all-MiniLM-L6-v2, dim: 384"
        else
            log_warn "embed-svc responding but unexpected health output: $health_json"
            warnings=$((warnings + 1))
        fi
    fi

    # 3. hooks-store health + Milvus connectivity
    if [ "${STATUS[hooks-store]:-}" = "running" ] || [ "${STATUS[hooks-store]:-}" = "started" ]; then
        local store_stats
        store_stats=$(curl -sf --max-time 3 http://localhost:9800/stats 2>/dev/null) || store_stats=""
        if [ -n "$store_stats" ]; then
            local errs
            errs=$(echo "$store_stats" | python3 -c "import sys,json; print(json.load(sys.stdin).get('errors',0))" 2>/dev/null) || errs="?"
            log_ok "hooks-store healthy — errors: $errs"
        else
            log_warn "hooks-store /health OK but /stats not responding"
            warnings=$((warnings + 1))
        fi
    fi

    # 4. UDS sockets exist (hooks-store was started by us with socket flags)
    if [ "${STATUS[hooks-store]:-}" = "started" ]; then
        # We started it — sockets should exist
        if [ -S /tmp/hooks-store.sock ]; then
            log_ok "hooks-store UDS ingest socket present"
        else
            log_warn "hooks-store UDS ingest socket missing (/tmp/hooks-store.sock)"
            warnings=$((warnings + 1))
        fi
        if [ -S /tmp/hooks-store-pub.sock ]; then
            log_ok "hooks-store pub socket present (monitor can subscribe)"
        else
            log_warn "hooks-store pub socket missing (/tmp/hooks-store-pub.sock) — monitor won't receive events"
            warnings=$((warnings + 1))
        fi
    elif [ "${STATUS[hooks-store]:-}" = "running" ]; then
        # Pre-existing — check if sockets exist, warn if not
        if [ ! -S /tmp/hooks-store-pub.sock ]; then
            log_warn "hooks-store was already running but pub socket missing — monitor won't receive events"
            log_warn "  → Restart with: ./scripts/stop.sh && ./scripts/start.sh"
            warnings=$((warnings + 1))
        else
            log_ok "hooks-store pub socket present"
        fi
        if [ ! -S /tmp/hooks-store.sock ]; then
            log_warn "hooks-store was already running but ingest UDS socket missing"
            log_warn "  → hook-client daemon cannot forward events via UDS"
            warnings=$((warnings + 1))
        else
            log_ok "hooks-store UDS ingest socket present"
        fi
    fi

    # 5. hook-client daemon socket is alive
    if [ "${STATUS[hook-client]:-}" = "running" ] || [ "${STATUS[hook-client]:-}" = "started" ]; then
        if [ -S /tmp/hook-client.sock ]; then
            log_ok "hook-client daemon socket alive"
        else
            log_warn "hook-client reported running but socket gone"
            warnings=$((warnings + 1))
        fi
    fi

    # 6. hooks-mcp binary
    if [ -x "$HOME/.local/bin/hooks-mcp" ]; then
        log_ok "hooks-mcp binary installed"
    fi

    # 7. Claude Code hooks — detailed validation
    if ! command -v python3 >/dev/null 2>&1; then
        log_warn "python3 not found — cannot validate hooks configuration"
        warnings=$((warnings + 1))
    else
        local settings="$HOME/.claude/settings.json"
        if [ ! -f "$settings" ]; then
            log_warn "Claude settings.json not found at $settings"
            warnings=$((warnings + 1))
        else
            local hooks_info
            hooks_info=$(python3 -c "
import json,sys
d=json.load(open(sys.argv[1]))
hooks=d.get('hooks',{})
if not hooks:
    print('NONE')
    sys.exit(0)
types=sorted(hooks.keys())
shim_types=[]
other_cmds=set()
for t in types:
    for entry in hooks[t]:
        for h in entry.get('hooks',[]):
            cmd=h.get('command','')
            if 'hook-shim' in cmd:
                shim_types.append(t)
            elif cmd:
                other_cmds.add(cmd)
print(f'TOTAL:{len(types)}')
print(f'SHIM:{len(shim_types)}')
print(f'TYPES:{\"|\".join(types)}')
print(f'SHIM_TYPES:{\"|\".join(shim_types)}')
if other_cmds:
    print(f'OTHER:{\"|\".join(sorted(other_cmds))}')
" "$settings" 2>&1) || hooks_info="ERROR"

            if [ "$hooks_info" = "NONE" ]; then
                log_warn "No hooks configured — run: hook-client install-hooks --shim"
                warnings=$((warnings + 1))
            elif [ "$hooks_info" = "ERROR" ]; then
                log_warn "Could not parse hooks from settings.json"
                warnings=$((warnings + 1))
            else
                local total shim_count hook_types_display shim_types_display other_cmds_display
                total=$(echo "$hooks_info" | grep '^TOTAL:' | cut -d: -f2)
                shim_count=$(echo "$hooks_info" | grep '^SHIM:' | cut -d: -f2)
                hook_types_display=$(echo "$hooks_info" | grep '^TYPES:' | cut -d: -f2- | tr '|' ', ')
                shim_types_display=$(echo "$hooks_info" | grep '^SHIM_TYPES:' | cut -d: -f2- | tr '|' ', ')
                other_cmds_display=$(echo "$hooks_info" | grep '^OTHER:' | cut -d: -f2- | tr '|' ', ')

                log_ok "Hooks enabled: $total hook types registered"
                printf "         ${DIM}Types: %s${NC}\n" "$hook_types_display"

                if [ "$shim_count" -gt 0 ]; then
                    log_ok "hook-shim registered on $shim_count hook types"
                    # Verify shim binary is reachable
                    if [ -x "$HOME/.local/bin/hook-shim" ]; then
                        log_ok "hook-shim binary installed at ~/.local/bin/hook-shim"
                    elif command -v hook-shim >/dev/null 2>&1; then
                        log_ok "hook-shim binary found in PATH"
                    elif [ -x "$ROOT/hook-shim/target/release/hook-shim" ]; then
                        log_warn "hook-shim built but not installed — run: cp $ROOT/hook-shim/target/release/hook-shim ~/.local/bin/"
                        warnings=$((warnings + 1))
                    else
                        log_warn "hook-shim binary not found — build: cd hook-shim && cargo build --release"
                        warnings=$((warnings + 1))
                    fi
                else
                    log_warn "hook-shim not registered on any hook type"
                    warnings=$((warnings + 1))
                fi

                if [ -n "$other_cmds_display" ]; then
                    printf "         ${DIM}Other hook commands: %s${NC}\n" "$other_cmds_display"
                fi
            fi
        fi
    fi

    echo ""

    if [ "$warnings" -gt 0 ]; then
        log_warn "$warnings warning(s) — pipeline may not work correctly"
    else
        log_ok "Pipeline fully operational"
    fi
}

# --- Status summary ---

print_status() {
    echo ""
    printf "${BOLD}%-20s %-12s %s${NC}\n" "COMPONENT" "STATUS" "ENDPOINT"
    printf "%-20s %-12s %s\n" "─────────────────" "──────────" "────────────────────"

    local components=("Milvus" "embed-svc" "hooks-store" "hook-client" "hook-shim" "hooks-mcp")
    for c in "${components[@]}"; do
        local status="${STATUS[$c]:-unknown}"
        local port="${PORTS[$c]:-—}"
        local color="$NC"
        case "$status" in
            running|started|installed) color="$GREEN" ;;
            skipped)                   color="$YELLOW" ;;
            failed|unknown)            color="$RED" ;;
        esac
        printf "%-20s ${color}%-12s${NC} %s\n" "$c" "$status" "$port"
    done
    echo ""
}

# --- Monitor launch ---

ask_and_launch_monitor() {
    # Check if monitor binary exists, build if needed.
    if ! ensure_binary "hooks-monitor" "bin/monitor" "hooks-monitor"; then
        log_warn "Could not build hooks-monitor — skipping"
        return 0
    fi

    # Warn if pub socket is missing — monitor will have nothing to show.
    if [ ! -S /tmp/hooks-store-pub.sock ]; then
        log_warn "Pub socket not available — monitor will not receive live events"
    fi

    echo "How would you like to monitor events?"
    echo "  1) TUI mode (interactive tree view)"
    echo "  2) Console mode (scrolling log)"
    echo "  3) Skip (exit script)"
    echo ""
    read -rp "Choice [1/2/3]: " choice

    case "$choice" in
        1)
            log_info "Launching monitor in TUI mode (Ctrl+C to exit monitor)..."
            echo ""
            HOOKS_STORE_PUB_SOCK=/tmp/hooks-store-pub.sock \
                "$ROOT/hooks-monitor/bin/monitor" --ui
            ;;
        2)
            log_info "Launching monitor in console mode (Ctrl+C to exit monitor)..."
            echo ""
            HOOKS_STORE_PUB_SOCK=/tmp/hooks-store-pub.sock \
                "$ROOT/hooks-monitor/bin/monitor"
            ;;
        3|"")
            log_info "All services running. Monitor skipped."
            ;;
        *)
            log_warn "Invalid choice — exiting."
            ;;
    esac
}

# --- Main ---

main() {
    echo ""
    printf "${BOLD}hooks4claude — Pipeline Startup${NC}\n"
    echo "════════════════════════════════════════"
    echo ""

    validate_prerequisites

    printf "${BOLD}Starting services${NC}\n"
    echo "────────────────────────────────────────"

    check_or_start_milvus      || true
    check_or_start_embed_svc   || true
    check_or_start_hooks_store || true
    check_or_start_hook_client || true
    ensure_hook_shim
    ensure_hooks_registered
    check_hooks_mcp

    validate_pipeline
    print_status
    ask_and_launch_monitor
}

main "$@"
