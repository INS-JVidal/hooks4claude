#!/usr/bin/env bash
# Stop all hooks4claude pipeline services.
# Safe to run when some or all services are already stopped.
set -uo pipefail
# NOTE: no set -e — we handle errors per-command to avoid aborting on pkill race conditions.

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BOLD='\033[1m'
NC='\033[0m'

log_ok()   { printf "${GREEN}[STOP]${NC}  %s\n" "$1"; }
log_warn() { printf "${YELLOW}[WARN]${NC}  %s\n" "$1"; }
log_skip() { printf "        %s\n" "$1"; }
log_fail() { printf "${RED}[FAIL]${NC}  %s\n" "$1"; }

# stop_by_pattern NAME PGREP_PATTERN [TIMEOUT_S]
# Finds process by pattern, sends SIGTERM, waits for exit.
stop_by_pattern() {
    local name="$1" pattern="$2" timeout="${3:-5}"

    # Find PIDs matching pattern, excluding this script.
    local pids
    pids=$(pgrep -f "$pattern" 2>/dev/null | grep -v "^$$\$" || true)

    if [ -z "$pids" ]; then
        log_skip "$name not running"
        return 1
    fi

    local pid_list
    pid_list=$(echo "$pids" | tr '\n' ' ')
    printf "  Stopping %s (PID %s)..." "$name" "$pid_list"

    # Send SIGTERM to each PID individually (safer than pkill race).
    for pid in $pids; do
        kill "$pid" 2>/dev/null || true
    done

    # Wait for processes to exit.
    local deadline=$((SECONDS + timeout))
    local still_running=true
    while [ "$SECONDS" -lt "$deadline" ]; do
        still_running=false
        for pid in $pids; do
            if kill -0 "$pid" 2>/dev/null; then
                still_running=true
                break
            fi
        done
        if [ "$still_running" = false ]; then
            printf " ${GREEN}done${NC}\n"
            log_ok "$name stopped"
            return 0
        fi
        sleep 0.2
    done

    # Still running after timeout — escalate to SIGKILL.
    printf " ${YELLOW}timeout, sending SIGKILL${NC}\n"
    for pid in $pids; do
        kill -9 "$pid" 2>/dev/null || true
    done
    sleep 0.5

    # Final check.
    local any_alive=false
    for pid in $pids; do
        if kill -0 "$pid" 2>/dev/null; then
            any_alive=true
        fi
    done

    if [ "$any_alive" = true ]; then
        log_fail "$name: could not kill (PID $pid_list) — check manually"
        return 1
    else
        log_ok "$name force-killed"
        return 0
    fi
}

echo ""
printf "${BOLD}hooks4claude — Pipeline Shutdown${NC}\n"
echo "════════════════════════════════════════"
echo ""

# Shutdown order: reverse of startup (consumers first, then producers, then infra).

# --- 1. hooks-monitor (consumer/viewer) ---
stop_by_pattern "hooks-monitor" 'hooks-monitor/bin/monitor'

# --- 2. hook-client daemon ---
if stop_by_pattern "hook-client daemon" 'hook-client daemon'; then
    rm -f /tmp/hook-client.sock
fi

# --- 3. hooks-store ---
if stop_by_pattern "hooks-store" 'hooks-store/bin/hooks-store'; then
    rm -f /tmp/hooks-store.sock /tmp/hooks-store-pub.sock
fi

# --- 4. embed-svc ---
stop_by_pattern "embed-svc" 'embed-svc/target/release/embed-svc'

# --- 5. Milvus (Docker) — only with --all ---
if [ "${1:-}" = "--all" ]; then
    ROOT="$(cd "$(dirname "$0")/.." && pwd)"
    if docker compose -f "$ROOT/docker/docker-compose.yml" ps --status running 2>/dev/null | grep -q milvus; then
        printf "  Stopping Milvus (Docker)..."
        if docker compose -f "$ROOT/docker/docker-compose.yml" down 2>"$ROOT/logs/docker-compose-down.log"; then
            printf " ${GREEN}done${NC}\n"
            log_ok "Milvus (Docker) stopped"
        else
            printf " ${RED}failed${NC}\n"
            log_fail "Milvus: docker compose down failed (see logs/docker-compose-down.log)"
        fi
    else
        log_skip "Milvus not running"
    fi
else
    log_skip "Milvus skipped (use --all to include Docker services)"
fi

# --- Clean up stale sockets that may be left from crashes ---
for sock in /tmp/hook-client.sock /tmp/hooks-store.sock /tmp/hooks-store-pub.sock; do
    if [ -S "$sock" ] || [ -e "$sock" ]; then
        # Only remove if no process is using it.
        if ! fuser "$sock" >/dev/null 2>&1; then
            rm -f "$sock"
        fi
    fi
done

echo ""
printf "${GREEN}Done.${NC} Run ${BOLD}./scripts/start.sh${NC} to restart.\n"
