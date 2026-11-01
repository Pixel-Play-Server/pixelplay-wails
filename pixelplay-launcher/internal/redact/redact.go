package redact

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"pixelplay-launcher/internal/paths"
)

var (
	reModsFile = regexp.MustCompile(`(?i)(?:[/\\]|^)mods[/\\][^:\s"'<>|]+\.(?:jar(?:\.disabled)?|zip)`)
	homeOnce   sync.Once
	cachedHome string
)

func homeDir() string {
	homeOnce.Do(func() {
		cachedHome, _ = os.UserHomeDir()
	})
	return cachedHome
}

// Line replaces local filesystem paths so launcher UI / Discord / mclo.gs never leak them.
func Line(s string, extraPaths ...string) string {
	if s == "" {
		return s
	}
	for _, p := range secrets(extraPaths...) {
		if p == "" {
			continue
		}
		s = strings.ReplaceAll(s, p, "[path]")
	}
	s = reModsFile.ReplaceAllString(s, "/mods/[mod]")
	return s
}

// Bytes is Line for a whole buffer.
func Bytes(b []byte, extraPaths ...string) []byte {
	return []byte(Line(string(b), extraPaths...))
}

// PatchFileInPlace masks secrets with same-length tokens so a live log4j appender
// is less likely to desync when we rewrite latest.log while Minecraft has it open.
func PatchFileInPlace(path string, extraPaths ...string) {
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return
	}
	out := MaskSameLength(string(b), extraPaths...)
	if out == string(b) {
		return
	}
	_ = os.WriteFile(path, []byte(out), 0o600)
}

// MaskSameLength replaces known paths with a same-length placeholder.
func MaskSameLength(s string, extraPaths ...string) string {
	for _, p := range secrets(extraPaths...) {
		if p == "" || !strings.Contains(s, p) {
			continue
		}
		s = strings.ReplaceAll(s, p, sameLenMask(p))
	}
	return reModsFile.ReplaceAllStringFunc(s, sameLenMask)
}

func sameLenMask(p string) string {
	n := len(p)
	if n <= 2 {
		return strings.Repeat("*", n)
	}
	return "[" + strings.Repeat("*", n-2) + "]"
}

func secrets(extra ...string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || p == "/" || p == "." {
			return
		}
		variants := []string{
			p,
			filepath.Clean(p),
			filepath.ToSlash(p),
			filepath.ToSlash(filepath.Clean(p)),
		}
		if resolved, err := filepath.EvalSymlinks(p); err == nil && resolved != "" {
			variants = append(variants, resolved, filepath.ToSlash(resolved))
		}
		for _, v := range variants {
			if v == "" || v == "/" || v == "." {
				continue
			}
			v = strings.TrimRight(v, `/\`)
			if len(v) < 4 {
				continue
			}
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}

	add(paths.DataDir())
	add(paths.ProtectedRoot())
	add(homeDir())
	if v := os.Getenv("APPDATA"); v != "" {
		add(v)
	}
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		add(v)
	}
	if v := os.Getenv("USERPROFILE"); v != "" {
		add(v)
	}
	for _, p := range extra {
		add(p)
		if p != "" {
			add(filepath.Dir(p))
		}
	}

	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}
