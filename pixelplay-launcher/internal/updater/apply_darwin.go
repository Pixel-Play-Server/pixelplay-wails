//go:build darwin

package updater

import (
	"archive/zip"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// macOS updates are app bundles, never binaries to overwrite Contents/MacOS with.
func ApplyAndRelaunch(newPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if real, e := filepath.EvalSymlinks(exe); e == nil {
		exe = real
	}
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if filepath.Ext(bundle) != ".app" || filepath.Base(filepath.Dir(exe)) != "MacOS" {
		return fmt.Errorf("instala Pixel Play.app en una carpeta con permisos de escritura antes de actualizar")
	}
	z, err := zip.OpenReader(newPath)
	if err != nil {
		return fmt.Errorf("actualización macOS no es un ZIP válido: %w", err)
	}
	defer z.Close()
	var total uint64
	for _, f := range z.File {
		name := strings.TrimSuffix(f.Name, "/")
		total += f.UncompressedSize64
		if !filepath.IsLocal(name) || strings.Contains(name, "\\") || f.Mode()&os.ModeSymlink != 0 || total > 1<<30 {
			return fmt.Errorf("entrada inválida en la actualización macOS")
		}
	}
	stage, err := os.MkdirTemp(filepath.Dir(bundle), ".pixelplay-update-")
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(stage)
		}
	}()
	if output, e := exec.Command("/usr/bin/ditto", "-x", "-k", newPath, stage).CombinedOutput(); e != nil {
		return fmt.Errorf("extraer actualización: %s: %w", output, e)
	}
	incoming := filepath.Join(stage, "pixelplay-launcher.app")
	binary := filepath.Join(incoming, "Contents", "MacOS", "pixelplay-launcher")
	if output, e := exec.Command(binary, "--version").CombinedOutput(); e != nil || strings.TrimSpace(string(output)) == "" {
		return fmt.Errorf("el nuevo launcher no supera la prueba de arranque: %s", output)
	}
	if output, e := exec.Command("/usr/bin/codesign", "--verify", "--deep", "--strict", incoming).CombinedOutput(); e != nil {
		return fmt.Errorf("bundle inválido: %s", output)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	script := fmt.Sprintf(`#!/bin/bash
set -eu
for i in {1..150}; do
 if ! kill -0 %d 2>/dev/null; then break; fi
 sleep 0.2
done
if kill -0 %d 2>/dev/null; then exit 1; fi
old=%s
new=%s
backup=%s
mv "$old" "$backup"
if ! mv "$new" "$old"; then mv "$backup" "$old"; exit 1; fi
/usr/bin/open -n "$old"
`, os.Getpid(), os.Getpid(), quote(bundle), quote(incoming), quote(filepath.Join(stage, "previous.app")))
	scriptPath := filepath.Join(stage, "apply.sh")
	if err = os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		return err
	}
	cmd := exec.Command("/bin/bash", scriptPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err = cmd.Start(); err != nil {
		return err
	}
	keep = true
	return nil
}
