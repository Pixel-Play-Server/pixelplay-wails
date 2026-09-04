#!/usr/bin/env bash
# Publish Linux + Windows builds to the beta updater tree and the landing /dl/beta.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SSH_HOST="${PIXEL_SSH_HOST:-pixel}"
VER="${1:-5.0.3}"
NOTES="${2:-Beta 5.0.3: Discord RPC, ícono Pixel Play, canal beta.}"
PUB_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
REMOTE_ROOT="/srv/dl/pixelplay/releases-v2-beta"
PUBLIC="https://panel.pixelplay.gg/dl/pixelplay/releases-v2-beta"
LANDING_DL="/srv/launcher/downloads/beta"

LINUX_BIN="$ROOT/build/bin/pixelplay-launcher"
WIN_BIN="$ROOT/build/bin/pixelplay-launcher.exe"

if [[ ! -x "$LINUX_BIN" ]]; then
  echo "missing linux binary: $LINUX_BIN" >&2
  exit 1
fi
if [[ ! -f "$WIN_BIN" ]]; then
  echo "missing windows binary: $WIN_BIN" >&2
  exit 1
fi

write_manifest() {
  local target="$1" artifact="$2" out="$3"
  cat >"$out" <<EOF
{
  "version": "${VER}",
  "notes": "${NOTES}",
  "pub_date": "${PUB_DATE}",
  "url": "${PUBLIC}/${target}/${VER}/${artifact}",
  "signature": ""
}
EOF
}

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
write_manifest linux-x86_64 pixelplay-launcher "$TMP/linux.json"
write_manifest windows-x86_64 pixelplay-launcher.exe "$TMP/win.json"

echo "==> remote dirs"
ssh "$SSH_HOST" "mkdir -p \
  '${REMOTE_ROOT}/linux-x86_64/${VER}' \
  '${REMOTE_ROOT}/linux-x86_64/5.0.0' \
  '${REMOTE_ROOT}/linux-x86_64/5.0.1' \
  '${REMOTE_ROOT}/linux-x86_64/5.0.2' \
  '${REMOTE_ROOT}/linux-x86_64/latest' \
  '${REMOTE_ROOT}/windows-x86_64/${VER}' \
  '${REMOTE_ROOT}/windows-x86_64/5.0.2' \
  '${REMOTE_ROOT}/windows-x86_64/latest' \
  '${REMOTE_ROOT}/mac-arm/${VER}' \
  '${REMOTE_ROOT}/mac-arm/latest' \
  '${REMOTE_ROOT}/mac-intel/${VER}' \
  '${REMOTE_ROOT}/mac-intel/latest' \
  '${LANDING_DL}'"

echo "==> upload linux"
scp -q "$LINUX_BIN" "$SSH_HOST:${REMOTE_ROOT}/linux-x86_64/${VER}/pixelplay-launcher"
ssh "$SSH_HOST" "chmod 755 '${REMOTE_ROOT}/linux-x86_64/${VER}/pixelplay-launcher'"
scp -q "$TMP/linux.json" "$SSH_HOST:${REMOTE_ROOT}/linux-x86_64/${VER}/version.json"
scp -q "$TMP/linux.json" "$SSH_HOST:${REMOTE_ROOT}/linux-x86_64/latest/version.json"
scp -q "$TMP/linux.json" "$SSH_HOST:${REMOTE_ROOT}/linux-x86_64/5.0.0/version.json"
scp -q "$TMP/linux.json" "$SSH_HOST:${REMOTE_ROOT}/linux-x86_64/5.0.1/version.json"
scp -q "$TMP/linux.json" "$SSH_HOST:${REMOTE_ROOT}/linux-x86_64/5.0.2/version.json"

echo "==> upload windows"
scp -q "$WIN_BIN" "$SSH_HOST:${REMOTE_ROOT}/windows-x86_64/${VER}/pixelplay-launcher.exe"
ssh "$SSH_HOST" "chmod 755 '${REMOTE_ROOT}/windows-x86_64/${VER}/pixelplay-launcher.exe'"
scp -q "$TMP/win.json" "$SSH_HOST:${REMOTE_ROOT}/windows-x86_64/${VER}/version.json"
scp -q "$TMP/win.json" "$SSH_HOST:${REMOTE_ROOT}/windows-x86_64/latest/version.json"
scp -q "$TMP/win.json" "$SSH_HOST:${REMOTE_ROOT}/windows-x86_64/5.0.2/version.json"

echo "==> landing /dl/beta"
scp -q "$LINUX_BIN" "$SSH_HOST:${LANDING_DL}/pixelplay-launcher"
scp -q "$WIN_BIN" "$SSH_HOST:${LANDING_DL}/pixelplay-launcher.exe"
ssh "$SSH_HOST" "chmod 755 '${LANDING_DL}/pixelplay-launcher' '${LANDING_DL}/pixelplay-launcher.exe'"

echo "beta ${VER} published"
echo "  ${PUBLIC}/linux-x86_64/${VER}/pixelplay-launcher"
echo "  ${PUBLIC}/windows-x86_64/${VER}/pixelplay-launcher.exe"
echo "  https://launcher.pixelplay.gg/dl/beta/pixelplay-launcher"
echo "  https://launcher.pixelplay.gg/dl/beta/pixelplay-launcher.exe"
