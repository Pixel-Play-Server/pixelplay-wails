#!/usr/bin/env bash
# Publish Linux + Windows (+ Mac zip if present remotely) to the stable updater tree.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SSH_HOST="${PIXEL_SSH_HOST:-pixel}"
VER="${1:-6.0.3}"
NOTES="${2:-Pixel Play update}"
PUB_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
REMOTE_ROOT="/srv/dl/pixelplay/releases-v2"
PUBLIC="https://panel.pixelplay.gg/dl/pixelplay/releases-v2"
LANDING_DL="/srv/launcher/downloads"
PUSH_URL="${PIXELPLAY_PUSH_BROADCAST:-https://panel.pixelplay.gg/launcher-push/broadcast}"
PUSH_TOKEN="${PIXELPLAY_PUSH_TOKEN:-}"

LINUX_BIN="$ROOT/build/bin/pixelplay-launcher"
WIN_BIN="$ROOT/build/bin/pixelplay-launcher.exe"
WIN_SETUP="$ROOT/build/bin/pixelplay-launcher-amd64-installer.exe"

if [[ ! -x "$LINUX_BIN" ]]; then
  echo "missing linux binary: $LINUX_BIN" >&2
  exit 1
fi
if [[ ! -f "$WIN_BIN" ]]; then
  echo "missing windows binary: $WIN_BIN" >&2
  exit 1
fi
if [[ ! -f "$WIN_SETUP" ]]; then
  echo "missing NSIS installer: $WIN_SETUP (build with -nsis -installscope user)" >&2
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
# IMPORTANT: updater URL must be the drop-in .exe, NEVER the NSIS installer.
# NSIS is only published for the web landing (PixelPlaySetup.exe).

echo "==> remote dirs"
ssh "$SSH_HOST" "mkdir -p \
  '${REMOTE_ROOT}/linux-x86_64/${VER}' \
  '${REMOTE_ROOT}/linux-x86_64/latest' \
  '${REMOTE_ROOT}/windows-x86_64/${VER}' \
  '${REMOTE_ROOT}/windows-x86_64/latest' \
  '${REMOTE_ROOT}/mac-arm/${VER}' \
  '${REMOTE_ROOT}/mac-arm/latest' \
  '${LANDING_DL}'"

echo "==> upload linux"
scp -q "$LINUX_BIN" "$SSH_HOST:${REMOTE_ROOT}/linux-x86_64/${VER}/pixelplay-launcher"
ssh "$SSH_HOST" "chmod 755 '${REMOTE_ROOT}/linux-x86_64/${VER}/pixelplay-launcher'"
scp -q "$TMP/linux.json" "$SSH_HOST:${REMOTE_ROOT}/linux-x86_64/${VER}/version.json"
scp -q "$TMP/linux.json" "$SSH_HOST:${REMOTE_ROOT}/linux-x86_64/latest/version.json"

echo "==> upload windows"
scp -q "$WIN_BIN" "$SSH_HOST:${REMOTE_ROOT}/windows-x86_64/${VER}/pixelplay-launcher.exe"
scp -q "$WIN_SETUP" "$SSH_HOST:${REMOTE_ROOT}/windows-x86_64/${VER}/pixelplay-launcher-amd64-installer.exe"
ssh "$SSH_HOST" "chmod 755 '${REMOTE_ROOT}/windows-x86_64/${VER}/pixelplay-launcher.exe' '${REMOTE_ROOT}/windows-x86_64/${VER}/pixelplay-launcher-amd64-installer.exe'"
scp -q "$TMP/win.json" "$SSH_HOST:${REMOTE_ROOT}/windows-x86_64/${VER}/version.json"
scp -q "$TMP/win.json" "$SSH_HOST:${REMOTE_ROOT}/windows-x86_64/latest/version.json"

echo "==> backfill ALL version folders so old clients can update"
# Copy tip version.json into every existing numbered folder for both platforms.
ssh "$SSH_HOST" bash -s -- "$REMOTE_ROOT" <<'REMOTE'
set -euo pipefail
ROOT="$1"
for plat in linux-x86_64 windows-x86_64; do
  tip="$ROOT/$plat/latest/version.json"
  [[ -f "$tip" ]] || continue
  for d in "$ROOT/$plat"/*/; do
    base="$(basename "$d")"
    [[ "$base" == "latest" ]] && continue
    cp -f "$tip" "$d/version.json"
  done
  echo "backfilled $plat"
done
REMOTE

echo "==> copy mac zip from beta if present"
ssh "$SSH_HOST" bash -s <<REMOTE
set -euo pipefail
VER='${VER}'
PUB_DATE='${PUB_DATE}'
PUBLIC='${PUBLIC}'
REMOTE_ROOT='${REMOTE_ROOT}'
MAC_BETA="/srv/dl/pixelplay/releases-v2-beta/mac-arm/\${VER}"
MAC_ST="\${REMOTE_ROOT}/mac-arm/\${VER}"
mkdir -p "\$MAC_ST" "\${REMOTE_ROOT}/mac-arm/latest"
shopt -s nullglob
zips=("\$MAC_BETA"/*.zip)
if ((\${#zips[@]}==0)); then
  echo "no mac zip on beta, skip"
  exit 0
fi
cp -f "\${zips[0]}" "\$MAC_ST/"
ART=\$(basename "\${zips[0]}")
cat > "\$MAC_ST/version.json" <<EOF
{
  "version": "\${VER}",
  "notes": "Pixel Play \${VER}",
  "pub_date": "\${PUB_DATE}",
  "url": "\${PUBLIC}/mac-arm/\${VER}/\${ART}",
  "signature": ""
}
EOF
cp -f "\$MAC_ST/version.json" "\${REMOTE_ROOT}/mac-arm/latest/version.json"
for d in "\${REMOTE_ROOT}/mac-arm"/*/; do
  base="\$(basename "\$d")"
  [[ "\$base" == "latest" ]] && continue
  cp -f "\$MAC_ST/version.json" "\$d/version.json"
done
echo "mac copied: \$ART"
REMOTE

echo "==> landing /dl"
scp -q "$LINUX_BIN" "$SSH_HOST:${LANDING_DL}/pixelplay-launcher"
scp -q "$WIN_BIN" "$SSH_HOST:${LANDING_DL}/pixelplay-launcher.exe"
scp -q "$WIN_SETUP" "$SSH_HOST:${LANDING_DL}/pixelplay-launcher-setup.exe"
scp -q "$WIN_SETUP" "$SSH_HOST:${LANDING_DL}/PixelPlaySetup.exe"
ssh "$SSH_HOST" "chmod 755 '${LANDING_DL}/pixelplay-launcher' '${LANDING_DL}/pixelplay-launcher.exe' '${LANDING_DL}/pixelplay-launcher-setup.exe' '${LANDING_DL}/PixelPlaySetup.exe'"

echo "==> push to connected launchers"
# No platform URL — clients re-check their own manifest (avoids Linux URL on Windows).
PAYLOAD=$(cat <<EOF
{"type":"update_available","from":"","to":"${VER}","notes":"${NOTES}","message":"Nueva version ${VER} en el canal estable"}
EOF
)
if [[ -n "$PUSH_TOKEN" ]]; then
  curl -sS -X POST "${PUSH_URL}" \
    -H "Content-Type: application/json" \
    -H "X-Pixel-Token: ${PUSH_TOKEN}" \
    -d "${PAYLOAD}" || true
else
  echo "No PIXELPLAY_PUSH_TOKEN configured: skipping push notifications."
fi
echo

echo "stable ${VER} published"
echo "  ${PUBLIC}/linux-x86_64/${VER}/pixelplay-launcher"
echo "  ${PUBLIC}/windows-x86_64/${VER}/pixelplay-launcher.exe"
echo "  ${PUBLIC}/windows-x86_64/${VER}/pixelplay-launcher-amd64-installer.exe"
echo "  https://launcher.pixelplay.gg/dl/PixelPlaySetup.exe"
echo "  https://launcher.pixelplay.gg/dl/pixelplay-launcher"
