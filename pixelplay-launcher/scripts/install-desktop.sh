#!/usr/bin/env bash
# Install / refresh Pixel Play .desktop with the original Pixel Play icon.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="${PIXELPLAY_BIN:-$ROOT/build/bin/pixelplay-launcher}"
ICONS="$ROOT/build/icons"
APP_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
HICOLOR="${XDG_DATA_HOME:-$HOME/.local/share}/icons/hicolor"
DESKTOP_NAME="pixelplay-launcher.desktop"
# Keep the old Go-testing desktop name as an alias so existing shortcuts still work.
LEGACY_DESKTOP_NAME="pixelplay-launcher-go.desktop"
DESKTOP_PATH="$APP_DIR/$DESKTOP_NAME"
DESKTOP_LINK="$HOME/Desktop/$DESKTOP_NAME"

if [[ ! -x "$BIN" ]]; then
  echo "error: binary not found or not executable: $BIN" >&2
  exit 1
fi

install_icon() {
  local size="$1" src="$2"
  local dest_dir="$HICOLOR/${size}x${size}/apps"
  mkdir -p "$dest_dir"
  if [[ -f "$src" ]]; then
    cp -f "$src" "$dest_dir/pixelplay-launcher.png"
  fi
}

mkdir -p "$APP_DIR"
install_icon 32  "$ICONS/32x32.png"
install_icon 128 "$ICONS/128x128.png"
install_icon 256 "$ICONS/256x256.png"
install_icon 512 "$ICONS/512x512.png"

# Fallback if the sized set is missing: use Wails appicon.
if [[ ! -f "$HICOLOR/512x512/apps/pixelplay-launcher.png" && -f "$ROOT/build/appicon.png" ]]; then
  install_icon 512 "$ROOT/build/appicon.png"
fi

STAMP="$(date -Iseconds)"

write_desktop() {
  local path="$1"
  cat >"$path" <<EOF
[Desktop Entry]
Version=1.0
Type=Application
Name=Pixel Play
GenericName=Minecraft Launcher
Comment=Launcher oficial de Pixel Play
Exec=$BIN
Icon=pixelplay-launcher
Path=$ROOT
Terminal=false
Categories=Game;
StartupNotify=true
StartupWMClass=pixelplay-launcher
Keywords=minecraft;pixelplay;launcher;
EOF
  chmod 644 "$path"
}

write_desktop "$DESKTOP_PATH"
write_desktop "$APP_DIR/$LEGACY_DESKTOP_NAME"

if [[ -d "$HOME/Desktop" ]]; then
  cp -f "$DESKTOP_PATH" "$DESKTOP_LINK"
  chmod +x "$DESKTOP_LINK" 2>/dev/null || true
  # Refresh the old testing shortcut if it is still on the Desktop.
  if [[ -f "$HOME/Desktop/$LEGACY_DESKTOP_NAME" ]]; then
    cp -f "$APP_DIR/$LEGACY_DESKTOP_NAME" "$HOME/Desktop/$LEGACY_DESKTOP_NAME"
    chmod +x "$HOME/Desktop/$LEGACY_DESKTOP_NAME" 2>/dev/null || true
  fi
fi

if command -v gtk-update-icon-cache >/dev/null 2>&1; then
  gtk-update-icon-cache -f "$HICOLOR" >/dev/null 2>&1 || true
fi
if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database "$APP_DIR" >/dev/null 2>&1 || true
fi

echo "desktop refreshed ($STAMP):"
echo "  $DESKTOP_PATH"
[[ -f "$DESKTOP_LINK" ]] && echo "  $DESKTOP_LINK"
echo "  Exec=$BIN"
echo "  Icon=pixelplay-launcher"
