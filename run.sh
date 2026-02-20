#!/bin/bash
# VRC VRPoker Stats launcher for Linux/Wayland
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY="$SCRIPT_DIR/vrpoker-stats"

# Build if binary doesn't exist or source is newer
if [ ! -f "$BINARY" ]; then
    echo "Building vrpoker-stats..."
    if command -v mise &>/dev/null; then
        mise run build
    else
        # Fallback: use Go directly
        GO_BIN="$(mise where go 2>/dev/null)/bin/go"
        [ -x "$GO_BIN" ] || GO_BIN="$(which go 2>/dev/null || echo '')"
        [ -x "$GO_BIN" ] || { echo "Error: Go not found. Install mise or Go manually."; exit 1; }
        "$GO_BIN" build -tags wayland -ldflags "-s -w" -o "$BINARY" "$SCRIPT_DIR/"
    fi
fi

exec "$BINARY" "$@"
