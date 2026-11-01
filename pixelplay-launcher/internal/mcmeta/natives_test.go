package mcmeta

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUnzipNativesFlattensArchPath(t *testing.T) {
	tmp := t.TempDir()
	jar := filepath.Join(tmp, "natives.jar")
	dest := filepath.Join(tmp, "out")
	_ = os.MkdirAll(dest, 0o755)

	osDir := HostNativeOSDir()
	arch := HostNativeArch()
	wrongArch := "x86"
	if arch == "x86" {
		wrongArch = "x64"
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, body string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	add("META-INF/MANIFEST.MF", "Manifest-Version: 1.0\n")
	for _, core := range expectedCoreNatives() {
		add(osDir+"/"+arch+"/org/lwjgl/"+core, "GOOD")
		add(osDir+"/"+wrongArch+"/org/lwjgl/"+core, "BAD")
	}
	core := expectedCoreNative()
	add("META-INF/"+osDir+"/"+arch+"/org/lwjgl/"+core+".sha1", "deadbeef")
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jar, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := unzipNatives(jar, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, core))
	if err != nil {
		t.Fatalf("core native missing: %v", err)
	}
	if string(got) != "GOOD" {
		t.Fatalf("got %q want GOOD (wrong arch leaked?)", got)
	}
	// Must not leave nested layout as the only copy java.library.path can see.
	if nativesHealthy(dest) != true {
		t.Fatal("expected healthy natives dir")
	}
}

func TestNativeClassifierWanted(t *testing.T) {
	osDir := HostNativeOSDir()
	arch := HostNativeArch()

	want := map[string]bool{
		"org.lwjgl:lwjgl:3.3.3:natives-" + osDir:                 arch == "x64",
		"org.lwjgl:lwjgl:3.3.3:natives-" + osDir + "-arm64":      arch == "arm64",
		"org.lwjgl:lwjgl:3.3.3:natives-" + osDir + "-x86":        arch == "x86",
		"org.lwjgl:lwjgl:3.3.3:natives-windows":                  osDir == "windows" && arch == "x64",
		"org.lwjgl:lwjgl:3.3.3:natives-linux":                    osDir == "linux" && arch == "x64",
		"org.lwjgl:lwjgl:3.3.3:natives-macos":                    osDir == "macos" && arch == "x64",
		"org.lwjgl:lwjgl:3.3.3:natives-windows-arm64":            osDir == "windows" && arch == "arm64",
		"cpw.mods:modlauncher:11.0.5":                            true,
	}
	// Cross-OS must be false
	if osDir != "windows" {
		want["org.lwjgl:lwjgl:3.3.3:natives-windows"] = false
	}
	if osDir != "linux" {
		want["org.lwjgl:lwjgl:3.3.3:natives-linux"] = false
	}

	for name, expect := range want {
		if got := NativeClassifierWanted(name); got != expect {
			t.Errorf("%s: got %v want %v (os=%s arch=%s goos=%s goarch=%s)",
				name, got, expect, osDir, arch, runtime.GOOS, runtime.GOARCH)
		}
	}
}

func TestCurrentArchNotX86OnAmd64(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("amd64 only")
	}
	if CurrentArch() != "x86_64" {
		t.Fatalf("CurrentArch=%q want x86_64", CurrentArch())
	}
}
