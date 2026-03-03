#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
INSTALL_DIR="${HOME}/.local/bin"
CONFIG_DIR="${HOME}/.config/claude-hooks-monitor"

cd "$PROJECT_DIR"

# --- Step 1: Build Docker images (compiles Go inside container) ---
# --profile full is required because the monitor service is profile-gated.
echo "Building Docker images..."
docker compose --profile full build

# --- Step 2: Extract hook-client binary from monitor image ---
echo "Extracting hook-client..."
mkdir -p "$INSTALL_DIR"
# Resolve the image name via docker compose (requires --profile full).
IMAGE=$(docker compose --profile full images monitor --format '{{.Repository}}:{{.Tag}}' 2>/dev/null | head -1)
if [ -z "$IMAGE" ] || [ "$IMAGE" = ":" ]; then
    # Fallback: conventional compose image naming.
    IMAGE="$(basename "$PROJECT_DIR")-monitor"
fi
CID=$(docker create "$IMAGE" 2>/dev/null)
if [ -z "$CID" ]; then
    echo "Error: could not create container from monitor image ($IMAGE)" >&2
    echo "  Ensure 'docker compose --profile full build' succeeded." >&2
    exit 1
fi
docker cp "$CID:/usr/local/bin/hook-client" "$INSTALL_DIR/hook-client"
docker rm "$CID" > /dev/null
chmod +x "$INSTALL_DIR/hook-client"
echo "Installed hook-client to $INSTALL_DIR/hook-client"

# Verify it's in PATH
if ! command -v hook-client &>/dev/null; then
    echo "Warning: $INSTALL_DIR is not in PATH."
    echo "Add to your shell profile: export PATH=\"$INSTALL_DIR:\$PATH\""
fi

# --- Step 3: Register hooks in ~/.claude/settings.json ---
echo "Registering hooks..."
"$INSTALL_DIR/hook-client" install-hooks

# --- Step 4: Configure sink forwarding ---
mkdir -p "$CONFIG_DIR"

if [ ! -f "$CONFIG_DIR/hook_monitor.conf" ]; then
    cp "$PROJECT_DIR/claude-hooks-monitor/hooks/hook_monitor.conf" "$CONFIG_DIR/hook_monitor.conf"
    # Enable sink forwarding (default config has forward = no)
    sed -i 's/^forward = no/forward = yes/' "$CONFIG_DIR/hook_monitor.conf"
    echo "Created config: $CONFIG_DIR/hook_monitor.conf (sink forwarding enabled)"
else
    echo "Config exists: $CONFIG_DIR/hook_monitor.conf"
    if ! grep -q "^forward = yes" "$CONFIG_DIR/hook_monitor.conf"; then
        echo "  Note: sink forwarding is disabled. To enable:"
        echo "  Edit $CONFIG_DIR/hook_monitor.conf and set: forward = yes"
    fi
fi

# --- Step 5: Write port file for Docker "full" profile ---
# When monitor runs in Docker (--profile full), hook-client needs to
# know the port. Write it to the XDG discovery location.
echo "8080" > "$CONFIG_DIR/.monitor-port"

echo ""
echo "Setup complete!"
echo ""
echo "Usage (recommended — TUI mode):"
echo "  docker compose up -d                              # Start backend"
echo "  cd claude-hooks-monitor && make run-ui             # Monitor with TUI"
echo ""
echo "Usage (headless — all Docker):"
echo "  docker compose --profile full up -d                # All services"
echo "  docker compose logs -f                             # View events"
