package java

import (
	"archive/tar"
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
	if emit != nil {
		emit(fmt.Sprintf("Buscando Java %d…", required))
	}
	if p, ok := findExisting(required); ok {
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
	if p, ok := findExisting(required); ok {
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
		candidates = append(candidates, filepath.Join(home, "bin", "java"))
	}
	if p, err := exec.LookPath("java"); err == nil {
		candidates = append(candidates, p)
	}
	max := maxCompatibleMajor(required)
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
	archive := destDir + ".tgz"
	if err := download.File(url, archive, nil); err != nil {
		return err
	}
	defer os.Remove(archive)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	return extractTarGz(archive, destDir)
}

func extractTarGz(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
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
