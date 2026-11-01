package modpackupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"pixelplay.gg/ppack"
)

func TestCheckDetectsNewer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"0.2.2","url":"http://example/x.ppack","sha256":"abc"}`))
	}))
	defer srv.Close()
	man, newer, err := Check("0.2.1", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !newer || man.Version != "0.2.2" {
		t.Fatalf("newer=%v man=%+v", newer, man)
	}
	_, newer, err = Check("0.2.2", srv.URL)
	if err != nil || newer {
		t.Fatalf("same version should not update: newer=%v err=%v", newer, err)
	}
	_, newer, err = Check("0.3.1", srv.URL)
	if err != nil || newer {
		t.Fatalf("older remote should not update: newer=%v err=%v", newer, err)
	}
}

func TestIsNewer(t *testing.T) {
	if !IsNewer("0.3.1", "0.2.0") {
		t.Fatal("0.3.1 should be newer than 0.2.0")
	}
	if IsNewer("0.3.1", "0.3.1") {
		t.Fatal("same version is not newer")
	}
	if IsNewer("0.2.0", "0.3.1") {
		t.Fatal("older remote is not newer")
	}
	if IsNewer("", "0.3.1") {
		t.Fatal("empty remote is not newer")
	}
}

func TestDownloadVerifiesSHA(t *testing.T) {
	body := []byte("ppack-bytes")
	sum := sha256.Sum256(body)
	sha := hex.EncodeToString(sum[:])
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	t.Setenv("PIXELPLAY_DATA_DIR", t.TempDir())
	dest, err := Download("pixelplay-survival-1211", &ppack.VersionManifest{
		Version: "0.2.2",
		URL:     srv.URL + "/pack.ppack",
		SHA256:  sha,
	})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dest) != "pixelplay-survival-1211-0.2.2.ppack" {
		t.Fatalf("name %s", dest)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != string(body) {
		t.Fatalf("content %q err=%v", got, err)
	}
}
