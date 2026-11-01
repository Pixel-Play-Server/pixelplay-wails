package java

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestForGameLaunchPrefersJavawOnWindows(t *testing.T) {
	dir := t.TempDir()
	javaPath := filepath.Join(dir, "java.exe")
	javawPath := filepath.Join(dir, "javaw.exe")
	if err := os.WriteFile(javaPath, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(javawPath, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := ForGameLaunch(javaPath)
	if runtime.GOOS == "windows" {
		if got != javawPath {
			t.Fatalf("got %q want %q", got, javawPath)
		}
	} else if got != javaPath {
		t.Fatalf("non-windows got %q", got)
	}
}
