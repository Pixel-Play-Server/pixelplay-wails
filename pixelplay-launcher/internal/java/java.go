package java

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"pixelplay-launcher/internal/download"
	"pixelplay-launcher/internal/paths"
)

func MetaJavaDir() string {
	return filepath.Join(paths.DataDir(), "meta", "java")
}

// maxCompatibleMajor is the newest JVM we will actually launch with for a
// given required major. Minecraft 1.21.x / NeoForge 21 need Java 21; Java 22+
// (especially 25) trips module ResolutionException on the game + DualCodecs.
// Newer Minecraft (26.x) asks for 25 and may use that.
func maxCompatibleMajor(required int) int {
	switch {
	case required <= 8:
		return 8
	case required <= 17:
		return 21
	case required <= 21:
		return 21
	default:
		return required
	}
}

type javaCand struct {
	path  string
	major int
}

func pickJava(cands []javaCand, required int) (string, bool) {
	if len(cands) == 0 {
		return "", false
	}
	best := cands[0]
	for _, c := range cands[1:] {
		exactC := c.major == required
		exactB := best.major == required
		if exactC && !exactB {
			best = c
			continue
		}
		if exactC == exactB && c.major < best.major {
			best = c
		}
	}
	return best.path, true
}

// Ensure returns a java binary matching required as closely as possible.
// It will not pick a JVM newer than maxCompatibleMajor(required); if only a
// too-new install exists (e.g. JDK 25 when the game needs 21), it downloads
// Temurin for the required major instead.
func Ensure(required int, emit func(string)) (string, error) {
	return ensureRange(required, maxCompatibleMajor(required), emit)
}

// EnsureExact is for runtimes such as Genesis that prescribe a JVM major.
func EnsureExact(required int, emit func(string)) (string, error) {
	return ensureRange(required, required, emit)
}

func MatchesMajor(bin string, required int) bool {
	major, err := versionOf(bin)
	return err == nil && major == required
}

func ensureRange(required, max int, emit func(string)) (string, error) {
	if emit != nil {
		emit(fmt.Sprintf("Buscando Java %d…", required))
	}
	if p, ok := findExistingRange(required, max); ok {
		if emit != nil {
			if major, err := versionOf(p); err == nil {
				emit(fmt.Sprintf("Usando Java %d", major))
			}
		}
		return p, nil
	}
	if emit != nil {
		emit(fmt.Sprintf("Descargando Temurin JDK %d…", required))
	}
	if err := downloadTemurin(required); err != nil {
		return "", err
	}
	if p, ok := findExistingRange(required, max); ok {
		return p, nil
	}
	return "", fmt.Errorf("Java %d no quedó instalado", required)
}

// Validate reports whether bin (or a JDK home) is a working java executable.
func Validate(bin string) bool {
	bin = strings.TrimSpace(bin)
	if bin == "" {
		return false
	}
	st, err := os.Stat(bin)
	if err != nil {
		return false
	}
	if st.IsDir() {
		cand := filepath.Join(bin, "bin", "java")
		if runtime.GOOS == "windows" {
			cand = filepath.Join(bin, "bin", "java.exe")
		}
		bin = cand
	}
	_, err = versionOf(bin)
	return err == nil
}

func findExisting(required int) (string, bool) {
	return findExistingRange(required, maxCompatibleMajor(required))
}

func findExistingRange(required, max int) (string, bool) {
	candidates := []string{}
	_ = filepath.Walk(MetaJavaDir(), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if info.Name() == "java" || info.Name() == "java.exe" {
			if strings.Contains(path, string(filepath.Separator)+"bin"+string(filepath.Separator)) {
				candidates = append(candidates, path)
			}
		}
		return nil
	})
	if home := os.Getenv("JAVA_HOME"); home != "" {
		if runtime.GOOS == "windows" {
			candidates = append(candidates, filepath.Join(home, "bin", "java.exe"))
		}
		candidates = append(candidates, filepath.Join(home, "bin", "java"))
	}
	if p, err := exec.LookPath("java"); err == nil {
		candidates = append(candidates, p)
	}
	if runtime.GOOS == "windows" {
		if p, err := exec.LookPath("java.exe"); err == nil {
			candidates = append(candidates, p)
		}
	}
	var ok []javaCand
	seen := map[string]bool{}
	for _, c := range candidates {
		if seen[c] {
			continue
		}
		seen[c] = true
		major, err := versionOf(c)
		if err != nil {
			continue
		}
		if major >= required && major <= max {
			ok = append(ok, javaCand{path: c, major: major})
		}
	}
	return pickJava(ok, required)
}

// ForGameLaunch prefers javaw.exe on Windows so Minecraft/Lunar does not open a
// fullscreen console window (java.exe is a console subsystem binary).
func ForGameLaunch(bin string) string {
	bin = strings.TrimSpace(bin)
	if bin == "" || runtime.GOOS != "windows" {
		return bin
	}
	base := strings.ToLower(filepath.Base(bin))
	if base == "javaw.exe" || base == "javaw" {
		return bin
	}
	dir := filepath.Dir(bin)
	for _, name := range []string{"javaw.exe", "javaw"} {
		cand := filepath.Join(dir, name)
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand
		}
	}
	return bin
}

var verRe = regexp.MustCompile(`version "([0-9]+)`)

func versionOf(bin string) (int, error) {
	cmd := exec.Command(bin, "-version")
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return 0, err
	}
	m := verRe.FindSubmatch(out)
	if m == nil {
		// Java 1.8.0_xxx
		re8 := regexp.MustCompile(`version "1\.([0-9]+)`)
		m = re8.FindSubmatch(out)
		if m == nil {
			return 0, fmt.Errorf("no pude leer versión de %s", bin)
		}
		n, _ := strconv.Atoi(string(m[1]))
		return n, nil
	}
	n, _ := strconv.Atoi(string(m[1]))
	return n, nil
}

func downloadTemurin(major int) error {
	osName := runtime.GOOS
	if osName == "darwin" {
		osName = "mac"
	}
	arch := "x64"
	if runtime.GOARCH == "arm64" {
		arch = "aarch64"
	}
	url := fmt.Sprintf(
		"https://api.adoptium.net/v3/binary/latest/%d/ga/%s/%s/jdk/hotspot/normal/eclipse?project=jdk",
		major, osName, arch,
	)
	destDir := filepath.Join(MetaJavaDir(), fmt.Sprintf("jdk-%d", major))
	_ = os.RemoveAll(destDir)
	// Windows Adoptium = .zip; Linux/macOS = .tar.gz. Sniff magic after download too.
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	archive := destDir + ext
	if err := download.File(url, archive, nil); err != nil {
		return err
	}
	defer os.Remove(archive)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	return extractJDKArchive(archive, destDir)
}

func extractJDKArchive(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	var magic [4]byte
	n, _ := io.ReadFull(f, magic[:])
	_, _ = f.Seek(0, io.SeekStart)
	f.Close()
	if n >= 2 && magic[0] == 'P' && magic[1] == 'K' {
		return extractZip(src, dest)
	}
	if n >= 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		return extractTarGz(src, dest)
	}
	// Fallback by extension / OS.
	low := strings.ToLower(src)
	if strings.HasSuffix(low, ".zip") || runtime.GOOS == "windows" {
		return extractZip(src, dest)
	}
	return extractTarGz(src, dest)
}

func extractZip(src, dest string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("abrir jdk zip: %w", err)
	}
	defer zr.Close()
	var top string
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		if top == "" {
			parts := strings.SplitN(name, "/", 2)
			top = parts[0]
		}
		rel := strings.TrimPrefix(name, top+"/")
		if rel == name {
			rel = name
		}
		if rel == "" {
			continue
		}
		target := filepath.Join(dest, filepath.FromSlash(rel))
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)+string(os.PathSeparator)) &&
			filepath.Clean(target) != filepath.Clean(dest) {
			return fmt.Errorf("path traversal en jdk zip")
		}
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(target, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTarGz(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip jdk (¿archivo zip en Windows?): %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var top string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.ToSlash(hdr.Name)
		if top == "" {
			parts := strings.SplitN(name, "/", 2)
			top = parts[0]
		}
		rel := strings.TrimPrefix(name, top+"/")
		if rel == name {
			rel = name
		}
		target := filepath.Join(dest, rel)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)+string(os.PathSeparator)) &&
			filepath.Clean(target) != filepath.Clean(dest) {
			return fmt.Errorf("path traversal en jdk")
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			_ = os.MkdirAll(target, 0o755)
		case tar.TypeReg:
			_ = os.MkdirAll(filepath.Dir(target), 0o755)
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			_, err = io.Copy(out, tr)
			out.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}
