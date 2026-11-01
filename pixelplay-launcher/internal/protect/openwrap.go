package protect

import (
	"os"
	"path/filepath"
	"strings"
)

const xdgOpenWrap = `#!/bin/sh
real=/usr/bin/xdg-open
[ -x /bin/xdg-open ] && real=/bin/xdg-open
if [ ! -x "$real" ]; then
  exit 0
fi
g=${PIXELPLAY_GAME_DIR-}
blocked() {
  t=$1
  [ -z "$g" ] && return 1
  case "$t" in
    http://*|https://*|mailto:*|steam:*) return 1 ;;
  esac
  p=$t
  case "$p" in
    file://*) p=${p#file://} ;;
    file:*) p=${p#file:} ;;
  esac
  p=${p%%\?*}
  case "$p" in
    /*) ;;
    *) p="$(pwd)/$p" ;;
  esac
  case "$p" in
    "$g"|"$g"/*) return 0 ;;
  esac
  if command -v realpath >/dev/null 2>&1; then
    r=$(realpath -m "$p" 2>/dev/null) || r=$p
    case "$r" in
      "$g"|"$g"/*) return 0 ;;
    esac
  fi
  return 1
}
for a in "$@"; do
  if blocked "$a"; then
    exit 0
  fi
done
exec "$real" "$@"
`

const gioWrap = `#!/bin/sh
real=/usr/bin/gio
[ -x "$real" ] || exit 0
g=${PIXELPLAY_GAME_DIR-}
blocked() {
  t=$1
  [ -z "$g" ] && return 1
  case "$t" in
    http://*|https://*|mailto:*|steam:*) return 1 ;;
  esac
  p=$t
  case "$p" in
    file://*) p=${p#file://} ;;
    file:*) p=${p#file:} ;;
  esac
  p=${p%%\?*}
  case "$p" in
    /*) ;;
    *) p="$(pwd)/$p" ;;
  esac
  case "$p" in
    "$g"|"$g"/*) return 0 ;;
  esac
  return 1
}
if [ "${1-}" = "open" ]; then
  shift
  for a in "$@"; do
    if blocked "$a"; then
      exit 0
    fi
  done
  exec "$real" open "$@"
fi
exec "$real" "$@"
`

// StageOpenWrappers writes decoy xdg-open/gio binaries at the front of PATH.
func StageOpenWrappers(runtimeDir string) (string, error) {
	if runtimeDir == "" {
		return "", nil
	}
	bin := filepath.Join(runtimeDir, ".bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		return "", err
	}
	for _, item := range []struct {
		name string
		body string
	}{
		{"xdg-open", xdgOpenWrap},
		{"gio", gioWrap},
		{"gnome-open", xdgOpenWrap},
		{"kde-open", xdgOpenWrap},
	} {
		p := filepath.Join(bin, item.name)
		if err := os.WriteFile(p, []byte(item.body), 0o755); err != nil {
			return "", err
		}
	}
	return bin, nil
}

// WithGuardEnv prepends guardBin to PATH and sets PIXELPLAY_GAME_DIR.
func WithGuardEnv(env []string, gameDir, guardBin string) []string {
	if env == nil {
		env = os.Environ()
	}
	out := make([]string, 0, len(env)+2)
	seenPath := false
	for _, e := range env {
		switch {
		case strings.HasPrefix(e, "PATH="):
			if guardBin != "" {
				out = append(out, "PATH="+guardBin+string(os.PathListSeparator)+strings.TrimPrefix(e, "PATH="))
			} else {
				out = append(out, e)
			}
			seenPath = true
		case strings.HasPrefix(e, "PIXELPLAY_GAME_DIR="):
			continue
		default:
			out = append(out, e)
		}
	}
	if !seenPath && guardBin != "" {
		out = append(out, "PATH="+guardBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	if gameDir != "" {
		out = append(out, "PIXELPLAY_GAME_DIR="+gameDir)
	}
	return out
}
