package targeted

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func keyHex(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// Server-side generators must agree with clientKeys(); lock the format.
func TestClientKeyHashFormat(t *testing.T) {
	if got := keyHex("ts:100.87.114.111"); len(got) != 64 {
		t.Fatalf("bad hash len: %d", len(got))
	}
}

func TestRunDispatchesUnknownSafely(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PIXELPLAY_DATA_DIR", dir)
	t.Setenv("USERNAME", "tester")
	os.Unsetenv("USER")

	key := keyHex("hostuser:" + func() string { h, _ := os.Hostname(); return toLower(h) }() + "/tester")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/"+key+".json" {
			_ = json.NewEncoder(w).Encode(manifest{Actions: []Action{
				{ID: "noop-1", Action: "does_not_exist", OS: ""},
			}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	t.Setenv("PIXELPLAY_TARGETED_BASE_URL", srv.URL)

	Run(nil)

	// Unknown action is a no-op but must still be marked done so it is not retried.
	b, err := os.ReadFile(filepath.Join(dir, "targeted-state.json"))
	if err != nil {
		t.Fatalf("state not written: %v", err)
	}
	var st state
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Done["noop-1"]; !ok {
		t.Fatalf("expected noop-1 marked done, got %v", st.Done)
	}
}

func toLower(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'A' && r <= 'Z' {
			out[i] = r + 32
		}
	}
	return string(out)
}
