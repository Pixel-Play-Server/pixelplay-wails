package filepick

import (
	"errors"
	"net/url"
	"strings"
)

// ErrUnsupported means the caller should use the Wails GTK/Cocoa/Win32 dialog.
var ErrUnsupported = errors.New("filepick: use native wails dialog")

func zenityFilter(filter string) string {
	filter = strings.ReplaceAll(strings.TrimSpace(filter), ";", " ")
	if filter == "" {
		return "*.ppack *.pixelplaypack *.mrpack *.zip"
	}
	return filter
}

// NormalizePath cleans dialog / drag-drop paths from Linux portals (file://, %20).
func NormalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return p
	}
	low := strings.ToLower(p)
	if strings.HasPrefix(low, "file://") {
		if u, err := url.Parse(p); err == nil && u.Path != "" {
			// url.Path is already decoded; on Unix host is empty for absolute paths
			p = u.Path
			if u.Host != "" && u.Host != "localhost" {
				p = "//" + u.Host + u.Path
			}
		} else {
			p = strings.TrimPrefix(p, "file://")
			p = strings.TrimPrefix(p, "FILE://")
			if un, err := url.PathUnescape(p); err == nil {
				p = un
			}
		}
	}
	return p
}

func filterLabel(title, filter string) string {
	blob := strings.ToLower(title + " " + zenityFilter(filter))
	switch {
	case strings.Contains(blob, "ppack") || strings.Contains(blob, "perfil") || strings.Contains(blob, "modpack"):
		return "Pixel Pack (.ppack)"
	case strings.Contains(blob, ".png"):
		return "Imágenes PNG"
	case strings.Contains(blob, ".jar"):
		return "Mods (.jar)"
	case title != "":
		return title
	default:
		return "Archivos"
	}
}
