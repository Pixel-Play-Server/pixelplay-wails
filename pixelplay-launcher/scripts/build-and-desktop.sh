#!/usr/bin/env bash
# Build Wails binary and refresh .desktop for local testing / feedback.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

TAGS="${WAILS_TAGS:-webkit2_41}"
SKIP_FRONTEND="${SKIP_FRONTEND:-0}"

echo "==> frontend"
if [[ "$SKIP_FRONTEND" != "1" ]]; then
  (cd frontend && npm run build)
fi

echo "==> wails build (-tags $TAGS)"
wails build -tags "$TAGS" -s

echo "==> install .desktop"
"$ROOT/scripts/install-desktop.sh"

echo "listo: $ROOT/build/bin/pixelplay-launcher"
