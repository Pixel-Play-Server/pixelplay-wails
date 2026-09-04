package java

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Installation matches the Rust JavaInstallation JSON the UI expects.
type Installation struct {
	Path          string  `json:"path"`
	Version       string  `json:"version"`
	MajorVersion  int     `json:"major_version"`
	Is64Bit       bool    `json:"is_64bit"`
	Vendor        string  `json:"vendor"`
	VMName        *string `json:"vm_name"`
	Source        string  `json:"source"`
	Architecture  string  `json:"architecture"`
	IsDefault     bool    `json:"is_default,omitempty"`
}

var (
	detectMu    sync.Mutex
	detectCache []Installation
	cacheValid  bool
)

var (
	fullVerRe  = regexp.MustCompile(`version "([^"]+)"`)
	vendorRe   = regexp.MustCompile(`(?i)(OpenJDK|Oracle|AdoptOpenJDK|Adoptium|Temurin|Azul|Eclipse|GraalVM|Amazon|Microsoft|BellSoft|Zulu|Corretto)`)
	vmRe       = regexp.MustCompile(`(?i)(HotSpot|OpenJ9|J9|GraalVM)`)
	bitRe      = regexp.MustCompile(`(?i)(64-Bit|x86_64|amd64|aarch64)`)
	armRe      = regexp.MustCompile(`(?i)(aarch64|arm64)`)
)

func InvalidateCache() {
	detectMu.Lock()
	defer detectMu.Unlock()
	cacheValid = false
	detectCache = nil
}

func DetectInstallations() ([]Installation, error) {
	detectMu.Lock()
	if cacheValid && detectCache != nil {
		out := append([]Installation(nil), detectCache...)
		detectMu.Unlock()
		return out, nil
	}
	detectMu.Unlock()

	found := map[string]Installation{}
	add := func(inst Installation) {
		key := filepath.Clean(inst.Path)
		if runtime.GOOS != "windows" {
			if resolved, err := filepath.EvalSymlinks(key); err == nil {
				key = resolved
				inst.Path = resolved
			}
		}
		if _, ok := found[key]; ok {
			return
		}
		found[key] = inst
	}

	tryPath := func(bin, source string) {
		info, err := Inspect(bin)
		if err != nil {
			return
		}
		info.Source = source
		add(*info)
	}

	// Launcher-managed Temurin JDKs
	_ = filepath.Walk(MetaJavaDir(), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if info.Name() == "java" || info.Name() == "java.exe" {
			if strings.Contains(path, string(filepath.Separator)+"bin"+string(filepath.Separator)) {
				tryPath(path, "Launcher Directory")
			}
		}
		return nil
	})

	if home := os.Getenv("JAVA_HOME"); home != "" {
		tryPath(javaBinInHome(home), "JAVA_HOME")
	}
	if p, err := exec.LookPath("java"); err == nil {
		tryPath(p, "PATH")
	}

	switch runtime.GOOS {
	case "linux":
		for _, dir := range linuxJavaRoots() {
			walkJavaHomes(dir, "Linux ("+dir+")", tryPath)
		}
		if _, err := os.Stat("/etc/arch-release"); err == nil {
			tryPath("/usr/lib/jvm/default/bin/java", "Arch Linux (default JVM)")
		}
		if out, err := exec.Command("update-alternatives", "--list", "java").Output(); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					tryPath(line, "update-alternatives")
				}
			}
		}
	case "darwin":
		for _, dir := range []string{
			"/Library/Java/JavaVirtualMachines",
			filepath.Join(os.Getenv("HOME"), "Library/Java/JavaVirtualMachines"),
			"/opt/homebrew/opt/openjdk/libexec/openjdk.jdk/Contents/Home",
			"/opt/homebrew/opt/openjdk@21/libexec/openjdk.jdk/Contents/Home",
			"/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home",
			"/usr/local/opt/openjdk/libexec/openjdk.jdk/Contents/Home",
		} {
			walkJavaHomes(dir, "macOS ("+dir+")", tryPath)
			tryPath(javaBinInHome(dir), "macOS ("+dir+")")
		}
		if out, err := exec.Command("/usr/libexec/java_home", "-V").CombinedOutput(); err == nil {
			// java_home -V prints to stderr, CombinedOutput captures it
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.TrimSpace(line)
				if i := strings.LastIndex(line, "/"); i >= 0 && strings.Contains(line, "Home") {
					home := strings.TrimSpace(line[strings.Index(line, "/"):])
					tryPath(javaBinInHome(home), "java_home")
				}
			}
		}
	case "windows":
		for _, dir := range windowsJavaRoots() {
			walkJavaHomes(dir, "Windows ("+dir+")", tryPath)
		}
	}

	list := make([]Installation, 0, len(found))
	for _, v := range found {
		list = append(list, v)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].MajorVersion != list[j].MajorVersion {
			return list[i].MajorVersion > list[j].MajorVersion
		}
		if list[i].Is64Bit != list[j].Is64Bit {
			return list[i].Is64Bit
		}
		return list[i].Path < list[j].Path
	})
	if len(list) > 0 {
		list[0].IsDefault = true
	}

	detectMu.Lock()
	detectCache = append([]Installation(nil), list...)
	cacheValid = true
	detectMu.Unlock()
	return list, nil
}

func Inspect(bin string) (*Installation, error) {
	bin = strings.TrimSpace(bin)
	if bin == "" {
		return nil, fmt.Errorf("ruta Java vacía")
	}
	st, err := os.Stat(bin)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		bin = javaBinInHome(bin)
	}
	cmd := exec.Command(bin, "-version")
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("java -version: %w", err)
	}
	text := string(out)
	m := fullVerRe.FindStringSubmatch(text)
	if m == nil {
		return nil, fmt.Errorf("no pude leer versión de %s", bin)
	}
	version := m[1]
	major, ok := parseMajor(version)
	if !ok {
		return nil, fmt.Errorf("versión Java inválida: %s", version)
	}
	vendor := "Unknown"
	if vm := vendorRe.FindString(text); vm != "" {
		vendor = vm
	}
	var vmName *string
	if vm := vmRe.FindString(text); vm != "" {
		v := vm
		vmName = &v
	}
	is64 := bitRe.MatchString(text)
	arch := "x86"
	if armRe.MatchString(text) {
		arch = "aarch64"
		is64 = true
	} else if is64 {
		arch = "x64"
	}
	return &Installation{
		Path:         bin,
		Version:      version,
		MajorVersion: major,
		Is64Bit:      is64,
		Vendor:       vendor,
		VMName:       vmName,
		Source:       "Manual",
		Architecture: arch,
	}, nil
}

func FindBestForMinecraft(mcVersion string) (*Installation, error) {
	list, err := DetectInstallations()
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	need := RequiredMajor(mcVersion)
	max := maxCompatibleMajor(need)
	var best *Installation
	for i := range list {
		j := list[i]
		if j.MajorVersion < need || j.MajorVersion > max {
			continue
		}
		if best == nil || j.MajorVersion < best.MajorVersion || (j.MajorVersion == best.MajorVersion && j.Is64Bit && !best.Is64Bit) {
			cp := j
			best = &cp
		}
	}
	if best != nil {
		return best, nil
	}
	return nil, nil
}

// RequiredMajor is the Java major Minecraft needs for that game version.
func RequiredMajor(mcVersion string) int {
	parts := strings.Split(strings.TrimSpace(mcVersion), ".")
	minor, patch := 0, 0
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) > 2 {
		patch, _ = strconv.Atoi(strings.Split(parts[2], "-")[0])
	}
	// 1.20.5+ → 21, 1.18+ → 17, 1.17 → 16, else 8
	if minor > 20 || (minor == 20 && patch >= 5) {
		return 21
	}
	if minor >= 18 {
		return 17
	}
	if minor >= 17 {
		return 16
	}
	return 8
}

func parseMajor(version string) (int, bool) {
	if strings.HasPrefix(version, "1.") {
		parts := strings.SplitN(version, ".", 3)
		if len(parts) >= 2 {
			n, err := strconv.Atoi(parts[1])
			return n, err == nil
		}
	}
	num := version
	for i, r := range version {
		if r < '0' || r > '9' {
			num = version[:i]
			break
		}
	}
	n, err := strconv.Atoi(num)
	return n, err == nil && n > 0
}

func javaBinInHome(home string) string {
	name := "java"
	if runtime.GOOS == "windows" {
		name = "java.exe"
	}
	// macOS bundle
	mac := filepath.Join(home, "Contents", "Home", "bin", name)
	if st, err := os.Stat(mac); err == nil && !st.IsDir() {
		return mac
	}
	return filepath.Join(home, "bin", name)
}

func walkJavaHomes(root, source string, try func(bin, source string)) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		p := filepath.Join(root, e.Name())
		if e.IsDir() || (e.Type()&os.ModeSymlink != 0) {
			try(javaBinInHome(p), source)
			try(javaBinInHome(filepath.Join(p, "Contents", "Home")), source)
		}
	}
}

func linuxJavaRoots() []string {
	home, _ := os.UserHomeDir()
	return []string{
		"/usr/lib/jvm",
		"/usr/lib64/jvm",
		"/usr/java",
		"/opt/java",
		"/opt/jdk",
		"/opt/openjdk",
		filepath.Join(home, ".sdkman/candidates/java"),
		filepath.Join(home, ".jdks"),
		filepath.Join(home, ".local/share/jdk"),
	}
}

func windowsJavaRoots() []string {
	pf := os.Getenv("ProgramFiles")
	pf86 := os.Getenv("ProgramFiles(x86)")
	local := os.Getenv("LOCALAPPDATA")
	roots := []string{}
	for _, base := range []string{pf, pf86} {
		if base == "" {
			continue
		}
		roots = append(roots,
			filepath.Join(base, "Java"),
			filepath.Join(base, "Eclipse Adoptium"),
			filepath.Join(base, "AdoptOpenJDK"),
			filepath.Join(base, "Microsoft"),
			filepath.Join(base, "Amazon Corretto"),
			filepath.Join(base, "Zulu"),
			filepath.Join(base, "BellSoft"),
			filepath.Join(base, "JavaSoft"),
		)
	}
	if local != "" {
		roots = append(roots, filepath.Join(local, "Programs", "Eclipse Adoptium"))
	}
	return roots
}
