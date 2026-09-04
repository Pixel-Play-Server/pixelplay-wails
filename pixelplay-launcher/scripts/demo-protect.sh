#!/usr/bin/env bash
# Demo rápida de protección de autor (sin UI).
set -euo pipefail
cd "$(dirname "$0")/.."

echo "=== Demo protección de autor ==="
go run ./cmd/demo-protect
