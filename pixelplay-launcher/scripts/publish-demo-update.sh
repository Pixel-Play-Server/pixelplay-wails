#!/usr/bin/env bash
# Publica un update en /srv/dl y EMPUJA un packet WebSocket a los launchers conectados.
set -euo pipefail

SSH_HOST="${PIXEL_SSH_HOST:-pixel}"
FROM_VER="${1:-0.2.0-wails}"
TO_VER="${2:-0.2.1-wails}"
TARGET="${3:-linux-x86_64}"
PUSH_URL="${PIXELPLAY_PUSH_BROADCAST:-https://panel.pixelplay.gg/launcher-push/broadcast}"
PUSH_TOKEN="${PIXELPLAY_PUSH_TOKEN:-pixelplay-push}"
ROOT="/srv/dl/pixelplay/releases-v2"
FROM_DIR="${ROOT}/${TARGET}/${FROM_VER}"
TO_DIR="${ROOT}/${TARGET}/${TO_VER}"
LATEST_DIR="${ROOT}/${TARGET}/latest"
ARTIFACT_NAME="pixelplay-launcher-${TO_VER}-demo.txt"
PUBLIC_BASE="https://panel.pixelplay.gg/dl/pixelplay/releases-v2/${TARGET}"
ARTIFACT_URL="${PUBLIC_BASE}/${TO_VER}/${ARTIFACT_NAME}"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

NOTES="Update ${TO_VER} publicada en Pixel. Push en vivo a launchers conectados."

cat >"${TMP}/${ARTIFACT_NAME}" <<EOF
Pixel Play Launcher update
version=${TO_VER}
published=$(date -u +%Y-%m-%dT%H:%M:%SZ)
host=panel.pixelplay.gg/dl
EOF

cat >"${TMP}/version.json" <<EOF
{
  "version": "${TO_VER}",
  "notes": "${NOTES}",
  "pub_date": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "url": "${ARTIFACT_URL}",
  "signature": ""
}
EOF

echo "==> 1) Subiendo artifacts a ${SSH_HOST}:${TO_DIR}"
ssh "$SSH_HOST" "mkdir -p '${FROM_DIR}' '${TO_DIR}' '${LATEST_DIR}'"
scp -q "${TMP}/${ARTIFACT_NAME}" "${SSH_HOST}:${TO_DIR}/${ARTIFACT_NAME}"
scp -q "${TMP}/version.json" "${SSH_HOST}:${FROM_DIR}/version.json"
scp -q "${TMP}/version.json" "${SSH_HOST}:${TO_DIR}/version.json"
scp -q "${TMP}/version.json" "${SSH_HOST}:${LATEST_DIR}/version.json"

echo "==> 2) Push packet a launchers conectados (${PUSH_URL})"
PAYLOAD=$(cat <<EOF
{"type":"update_available","from":"${FROM_VER}","to":"${TO_VER}","url":"${ARTIFACT_URL}","notes":"${NOTES}","message":"Nueva descarga publicada: ${TO_VER}"}
EOF
)
RESP=$(curl -sS -X POST "${PUSH_URL}?token=${PUSH_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Pixel-Token: ${PUSH_TOKEN}" \
  -d "${PAYLOAD}")
echo "    ${RESP}"

echo "==> 3) Manifest público:"
echo "    ${PUBLIC_BASE}/${FROM_VER}/version.json"
curl -sS "${PUBLIC_BASE}/${FROM_VER}/version.json"
echo
echo "Listo. Los launchers con WS conectado deberían notificar al toque."
