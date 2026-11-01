package mcmeta

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// HostNativeArch is the LWJGL / Mojang native folder name for this CPU
// (x64, arm64, or x86). Distinct from CurrentArch(), which follows Mojang
// rule strings (x86_64 / arm64 / x86).
func HostNativeArch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "arm64"
	case "386":
		return "x86"
	default:
		return "x64"
	}
}

// HostNativeOSDir is the top-level folder inside modern LWJGL natives jars
// (windows, linux, macos).
func HostNativeOSDir() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "macos"
	default:
		return "linux"
	}
}

func expectedCoreNatives() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"lwjgl.dll", "glfw.dll", "OpenAL.dll"}
	case "darwin":
		return []string{"liblwjgl.dylib", "libglfw.dylib", "libopenal.dylib"}
	default:
		return []string{"liblwjgl.so", "libglfw.so", "libopenal.so"}
	}
}

func expectedCoreNative() string {
	n := expectedCoreNatives()
	if len(n) == 0 {
		return "lwjgl"
	}
	return n[0]
}

func isNativeBinaryName(name string) bool {
	n := strings.ToLower(name)
	return strings.HasSuffix(n, ".dll") ||
		strings.HasSuffix(n, ".so") ||
		strings.HasSuffix(n, ".dylib") ||
		strings.HasSuffix(n, ".jnilib")
}

// nativesHealthy reports whether the flat core LWJGL natives are present.
func nativesHealthy(dest string) bool {
	for _, name := range expectedCoreNatives() {
		st, err := os.Stat(filepath.Join(dest, name))
		if err != nil || st.Size() == 0 {
			return false
		}
	}
	return true
}

// NativeClassifierWanted filters maven natives classifiers to this OS/arch
// (e.g. natives-windows vs natives-windows-x86 / natives-linux-arm64).
func NativeClassifierWanted(libName string) bool {
	parts := strings.Split(libName, ":")
	if len(parts) < 4 {
		return true
	}
	c := parts[3]
	if !strings.HasPrefix(c, "natives-") {
		return true
	}
	rest := strings.TrimPrefix(c, "natives-")
	osDir := HostNativeOSDir()
	// Mojang / LWJGL use "osx" in some classifiers.
	switch {
	case strings.HasPrefix(rest, "windows"):
		if osDir != "windows" {
			return false
		}
		rest = strings.TrimPrefix(rest, "windows")
	case strings.HasPrefix(rest, "linux"):
		if osDir != "linux" {
			return false
		}
		rest = strings.TrimPrefix(rest, "linux")
	case strings.HasPrefix(rest, "macos"):
		if osDir != "macos" {
			return false
		}
		rest = strings.TrimPrefix(rest, "macos")
	case strings.HasPrefix(rest, "osx"):
		if osDir != "macos" {
			return false
		}
		rest = strings.TrimPrefix(rest, "osx")
	default:
		return true
	}
	rest = strings.TrimPrefix(rest, "-")
	switch rest {
	case "":
		// natives-windows / natives-linux / natives-macos → default x64 jar
		return HostNativeArch() == "x64"
	case "x86":
		return HostNativeArch() == "x86"
	case "arm64", "aarch64":
		return HostNativeArch() == "arm64"
	case "x86_64", "amd64", "x64":
		return HostNativeArch() == "x64"
	default:
		return true
	}
}

func cleanupBadNativesLayout(dest string) {
	// Remove nested jar layouts and leftover META-INF that break java.library.path.
	for _, name := range []string{"windows", "linux", "macos", "osx", "META-INF"} {
		_ = os.RemoveAll(filepath.Join(dest, name))
	}
}

// EnsureNatives downloads (if needed) and extracts platform natives into a flat
// java.library.path directory — same auto-provision idea as java.Ensure (Temurin).
func EnsureNatives(meta *PistonMeta, extra []Library, emit Progress) error {
	if meta == nil || meta.ID == "" {
		return fmt.Errorf("natives: meta vacío")
	}
	dest := NativesDir(meta.ID)
	_ = os.MkdirAll(dest, 0o755)

	healthy := nativesHealthy(dest)
	if !healthy {
		if emit != nil {
			emit(fmt.Sprintf("Instalando natives (%s/%s)…", HostNativeOSDir(), HostNativeArch()))
		}
		cleanupBadNativesLayout(dest)
	}
	if err := ExtractNatives(meta, extra); err != nil {
		return err
	}
	if !nativesHealthy(dest) {
		if emit != nil {
			emit("Reintentando extracción de natives…")
		}
		cleanupBadNativesLayout(dest)
		if err := ExtractNatives(meta, extra); err != nil {
			return err
		}
	}
	if !nativesHealthy(dest) {
		return fmt.Errorf("natives incompletos en %s (falta lwjgl/glfw/OpenAL)", dest)
	}
	if emit != nil && !healthy {
		emit("Natives listos")
	}
	return nil
}

func unzipNatives(jar, dest string) error {
	zr, err := zip.OpenReader(jar)
	if err != nil {
		return err
	}
	defer zr.Close()

	osDir := HostNativeOSDir()
	arch := HostNativeArch()
	// Prefer entries under <os>/<arch>/…; also accept flat root binaries (legacy jars).
	prefixExact := osDir + "/" + arch + "/"
	prefixOS := osDir + "/"

	type cand struct {
		f     *zip.File
		score int
	}
	var picks []cand
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.ToSlash(f.Name)
		if strings.HasPrefix(name, "META-INF/") {
			continue
		}
		base := name
		if i := strings.LastIndex(name, "/"); i >= 0 {
			base = name[i+1:]
		}
		if !isNativeBinaryName(base) {
			continue
		}
		score := 0
		switch {
		case strings.HasPrefix(name, prefixExact):
			score = 3
		case !strings.Contains(name, "/"):
			score = 2 // legacy flat
		case strings.HasPrefix(name, prefixOS) && !strings.Contains(name, "/x86/") && !strings.Contains(name, "/arm64/") && !strings.Contains(name, "/aarch64/"):
			score = 1
		default:
			// Wrong OS/arch nested path (e.g. windows/x86 or linux/arm64 on x64 host).
			continue
		}
		picks = append(picks, cand{f: f, score: score})
	}

	best := map[string]cand{}
	for _, p := range picks {
		base := p.f.Name
		if i := strings.LastIndex(filepath.ToSlash(base), "/"); i >= 0 {
			base = base[i+1:]
		}
		if prev, ok := best[base]; ok && prev.score >= p.score {
			continue
		}
		best[base] = p
	}

	for _, p := range best {
		name := filepath.ToSlash(p.f.Name)
		base := name
		if i := strings.LastIndex(name, "/"); i >= 0 {
			base = name[i+1:]
		}
		target := filepath.Join(dest, base)
		if err := writeZipFile(p.f, target); err != nil {
			return err
		}
	}
	return nil
}

func writeZipFile(f *zip.File, target string) error {
	_ = os.MkdirAll(filepath.Dir(target), 0o755)
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, rc)
	cerr := out.Close()
	if err != nil {
		return err
	}
	return cerr
}
