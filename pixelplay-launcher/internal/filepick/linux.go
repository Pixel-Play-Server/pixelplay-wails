//go:build linux

package filepick

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func zenity() (string, error) {
	return exec.LookPath("zenity")
}

func cancel(err error) bool {
	if err == nil {
		return false
	}
	ee, ok := err.(*exec.ExitError)
	return ok && ee.ExitCode() == 1
}

func run(args ...string) (string, error) {
	bin, err := zenity()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if cancel(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return NormalizePath(strings.TrimSpace(string(out))), nil
}

func Open(title, filter string) (string, error) {
	if title == "" {
		title = "Seleccionar archivo"
	}
	label := filterLabel(title, filter)
	pat := zenityFilter(filter)
	return run(
		"--file-selection",
		"--title="+title,
		"--file-filter="+label+" | "+pat,
		"--file-filter=Todos los archivos | *",
	)
}

func OpenMany(title, filter string) ([]string, error) {
	if title == "" {
		title = "Seleccionar archivos"
	}
	label := filterLabel(title, filter)
	pat := zenityFilter(filter)
	raw, err := run(
		"--file-selection",
		"--multiple",
		"--separator=\n",
		"--title="+title,
		"--file-filter="+label+" | "+pat,
		"--file-filter=Todos | *",
	)
	if err != nil || raw == "" {
		return nil, err
	}
	parts := strings.Split(raw, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = NormalizePath(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out, nil
}

func OpenDir(title string) (string, error) {
	if title == "" {
		title = "Seleccionar carpeta"
	}
	return run("--file-selection", "--directory", "--title="+title)
}

func Save(title, suggested, filter string) (string, error) {
	if title == "" {
		title = "Guardar archivo"
	}
	if suggested == "" {
		suggested = "pack.ppack"
	}
	_ = os.MkdirAll(filepath.Dir(suggested), 0o755)
	label := filterLabel(title, filter)
	pat := zenityFilter(filter)
	args := []string{
		"--file-selection", "--save", "--confirm-overwrite",
		"--title=" + title,
		"--filename=" + suggested,
		"--file-filter=" + label + " | " + pat,
		"--file-filter=Todos | *",
	}
	return run(args...)
}
