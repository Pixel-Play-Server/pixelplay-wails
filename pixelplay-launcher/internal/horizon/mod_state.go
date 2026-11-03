package horizon

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var modStateMu sync.Mutex
var safeModID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

func ModEnabled(gameDir, id string) bool {
	b, _ := os.ReadFile(filepath.Join(gameDir, "horizon-mods.txt"))
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(k) == "mod."+id && strings.TrimSpace(v) == "true" {
			return true
		}
	}
	return false
}
func SetModEnabled(gameDir, id string, enabled bool) error {
	if !safeModID.MatchString(id) {
		return fmt.Errorf("id de mod inválido")
	}
	modStateMu.Lock()
	defer modStateMu.Unlock()
	path := filepath.Join(gameDir, "horizon-mods.txt")
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := []string{}
	for _, line := range strings.Split(string(b), "\n") {
		k, _, _ := strings.Cut(line, ":")
		if strings.TrimSpace(k) != "mod."+id && line != "" {
			lines = append(lines, line)
		}
	}
	lines = append(lines, fmt.Sprintf("mod.%s:%t", id, enabled))
	tmp, err := os.CreateTemp(gameDir, ".mods-state-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.WriteString(strings.Join(lines, "\n") + "\n")
	ce := tmp.Close()
	if err != nil {
		return err
	}
	if ce != nil {
		return ce
	}
	return os.Rename(tmp.Name(), path)
}
