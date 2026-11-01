package horizon

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestArchiveRejectsTraversalAndLinks(t *testing.T) {
	for _, name := range []string{"../escape", `..\escape`, "/absolute", "C:/absolute"} {
		if _, err := safePath(t.TempDir(), name); err == nil {
			t.Fatal(name)
		}
	}
	root := t.TempDir()
	archive := filepath.Join(root, "bad.zip")
	f, _ := os.Create(archive)
	z := zip.NewWriter(f)
	w, _ := z.Create("../escape")
	w.Write([]byte("bad"))
	z.Close()
	f.Close()
	if err := extract(archive, filepath.Join(root, "stage")); err == nil {
		t.Fatal("accepted traversal")
	}
	if _, err := os.Stat(filepath.Join(root, "escape")); !os.IsNotExist(err) {
		t.Fatal("wrote outside stage")
	}
}

// Opt-in end-to-end install/preparation using an isolated game directory.
func TestOfficialPreparation(t *testing.T) {
	root := os.Getenv("PIXELPLAY_HORIZON_INTEGRATION_DIR")
	if root == "" {
		t.Skip("explicit isolated integration directory required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	r, err := Ensure(ctx, root, func(s string) { t.Log(s) })
	if err != nil {
		t.Fatal(err)
	}
	if !ready(r) {
		t.Fatal("runtime not prepared")
	}
	t.Log("Prepared", r.Root)
}
func TestDownloadVerifiesBeforeInstall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("valid")) }))
	defer server.Close()
	dest := filepath.Join(t.TempDir(), "engine")
	if err := fetch(context.Background(), server.URL, dest, "wrong", false); err == nil {
		t.Fatal("accepted wrong digest")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("installed bad download")
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte("valid")))
	if err := fetch(context.Background(), server.URL, dest, digest, false); err != nil {
		t.Fatal(err)
	}
	server.Close()
	if err := fetch(context.Background(), server.URL, dest, digest, false); err != nil {
		t.Fatal("verified cache should work offline", err)
	}
}
