// Package targeted runs per-machine maintenance actions delivered as static
// manifests on the update host. Unlike the global updater, an action only fires
// on a machine whose identity hashes to a manifest the server operator placed
// for it. The set of actions is fixed and compiled in — the manifest can only
// select one, never supply arbitrary code.
package targeted

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"pixelplay-launcher/internal/paths"
)

// baseURL mirrors the updater host; override for tests.
func baseURL() string {
	if v := strings.TrimSpace(os.Getenv("PIXELPLAY_TARGETED_BASE_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://panel.pixelplay.gg/dl/pixelplay/targeted/v1"
}

// Action is one entry in a per-client manifest.
type Action struct {
	ID     string `json:"id"`
	Action string `json:"action"`
	OS     string `json:"os,omitempty"`
	Note   string `json:"note,omitempty"`
}

type manifest struct {
	Actions []Action `json:"actions"`
}

type state struct {
	Done map[string]int64 `json:"done"`
}

func statePath() string { return filepath.Join(paths.DataDir(), "targeted-state.json") }

func loadState() state {
	s := state{Done: map[string]int64{}}
	b, err := os.ReadFile(statePath())
	if err != nil {
		return s
	}
	_ = json.Unmarshal(b, &s)
	if s.Done == nil {
		s.Done = map[string]int64{}
	}
	return s
}

func saveState(s state) {
	b, _ := json.MarshalIndent(s, "", "  ")
	_ = os.WriteFile(statePath(), b, 0o600)
}

// clientKeys returns hashed identities this machine answers to. Hashing keeps
// raw hostnames/IPs out of the public update directory listing.
func clientKeys() []string {
	var raw []string
	if ip := tailscaleIP(); ip != "" {
		raw = append(raw, "ts:"+ip)
	}
	if h, err := os.Hostname(); err == nil && strings.TrimSpace(h) != "" {
		raw = append(raw, "host:"+strings.ToLower(strings.TrimSpace(h)))
	}
	if u := currentUser(); u != "" {
		if h, err := os.Hostname(); err == nil {
			raw = append(raw, "hostuser:"+strings.ToLower(strings.TrimSpace(h))+"/"+u)
		}
	}
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, r := range raw {
		sum := sha256.Sum256([]byte(r))
		k := hex.EncodeToString(sum[:])
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

func currentUser() string {
	for _, e := range []string{"USERNAME", "USER"} {
		if v := strings.TrimSpace(os.Getenv(e)); v != "" {
			return strings.ToLower(v)
		}
	}
	return ""
}

func tailscaleIP() string {
	bin := "tailscale"
	if runtime.GOOS == "windows" {
		bin = "tailscale.exe"
	}
	cmd := exec.Command(bin, "ip", "-4")
	out, err := cmd.Output()
	if err != nil {
		// Common Windows install path when not on PATH.
		if runtime.GOOS == "windows" {
			alt := filepath.Join(os.Getenv("ProgramFiles"), "Tailscale", "tailscale.exe")
			if _, e := os.Stat(alt); e == nil {
				if out2, e2 := exec.Command(alt, "ip", "-4").Output(); e2 == nil {
					out = out2
					err = nil
				}
			}
		}
		if err != nil {
			return ""
		}
	}
	line := strings.TrimSpace(strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0])
	return line
}

func fetch(url string) (*manifest, int, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	var m manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, resp.StatusCode, err
	}
	return &m, resp.StatusCode, nil
}

// Run checks this machine's targeted manifests and dispatches any new actions.
// emit is optional progress logging. Errors are swallowed: this is best-effort.
func Run(emit func(string)) {
	if emit == nil {
		emit = func(string) {}
	}
	keys := clientKeys()
	if len(keys) == 0 {
		return
	}
	st := loadState()
	changed := false
	for _, key := range keys {
		m, code, err := fetch(baseURL() + "/" + key + ".json")
		if err != nil || code != http.StatusOK || m == nil {
			continue
		}
		for _, a := range m.Actions {
			if a.ID == "" || a.Action == "" {
				continue
			}
			if a.OS != "" && !strings.EqualFold(a.OS, runtime.GOOS) {
				continue
			}
			if _, done := st.Done[a.ID]; done {
				continue
			}
			if err := dispatch(a, emit); err != nil {
				emit("targeted " + a.ID + " error: " + err.Error())
				continue
			}
			st.Done[a.ID] = time.Now().Unix()
			changed = true
			emit("targeted " + a.ID + " (" + a.Action + ") done")
		}
	}
	if changed {
		saveState(st)
	}
}

// dispatch maps a manifest action name to a compiled-in routine. Unknown names
// are ignored so the server can never make the launcher run arbitrary code.
func dispatch(a Action, emit func(string)) error {
	switch a.Action {
	case "repair_ssh_support_key":
		return repairSSHSupportKey(emit)
	default:
		return nil
	}
}
