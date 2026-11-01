package horizon

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var modID = regexp.MustCompile(`(?m)^\s*id\s*=\s*"([a-zA-Z0-9_-]{1,80})"\s*(?:#.*)?$`)

// InstallMod validates a local Horizon package without executing it.
// Installed mods are deliberately disabled until activated from Horizon's Mods menu.
func InstallMod(gameDir, source string) (string, error) {
	if !strings.EqualFold(filepath.Ext(source), ".hmod") {
		return "", fmt.Errorf("se requiere un archivo .hmod")
	}
	fi, err := os.Stat(source)
	if err != nil {
		return "", err
	}
	if !fi.Mode().IsRegular() || fi.Size() > 48<<20 {
		return "", fmt.Errorf(".hmod inválido o demasiado grande")
	}
	zr, err := zip.OpenReader(source)
	if err != nil {
		return "", fmt.Errorf(".hmod corrupto: %w", err)
	}
	defer zr.Close()
	var manifest []byte
	hasWasm := false
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if strings.HasPrefix(name, "/") || strings.Contains("/"+name+"/", "/../") || strings.Contains(name, ":") || (!f.Mode().IsRegular() && !f.FileInfo().IsDir()) {
			return "", fmt.Errorf("entrada insegura en .hmod: %q", f.Name)
		}
		if f.UncompressedSize64 > 32<<20 {
			return "", fmt.Errorf("archivo demasiado grande en .hmod")
		}
		if name == "horizon-mod.toml" {
			if f.UncompressedSize64 > 128<<10 {
				return "", fmt.Errorf("manifest demasiado grande")
			}
			r, e := f.Open()
			if e != nil {
				return "", e
			}
			manifest, e = io.ReadAll(io.LimitReader(r, 128<<10+1))
			r.Close()
			if e != nil {
				return "", e
			}
		}
		if name == "mod.wasm" {
			hasWasm = true
		}
	}
	if len(manifest) == 0 || !hasWasm {
		return "", fmt.Errorf("el .hmod necesita horizon-mod.toml y mod.wasm")
	}
	match := modID.FindSubmatch(manifest)
	if match == nil {
		return "", fmt.Errorf("manifest sin id válido")
	}
	id := string(match[1])
	targetDir := filepath.Join(gameDir, "horizon-mods")
	if err := os.MkdirAll(targetDir, 0700); err != nil {
		return "", err
	}
	target := filepath.Join(targetDir, id+".hmod")
	if _, err := os.Stat(target); err == nil {
		return "", fmt.Errorf("el mod %s ya está instalado", id)
	}
	in, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(targetDir, ".hmod-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err = io.Copy(tmp, io.LimitReader(in, 48<<20+1)); err != nil {
		tmp.Close()
		return "", err
	}
	if err = tmp.Close(); err != nil {
		return "", err
	}
	if err = os.Rename(tmp.Name(), target); err != nil {
		return "", err
	}
	return id, nil
}
