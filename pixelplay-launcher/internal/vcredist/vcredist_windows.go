//go:build windows

package vcredist

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"pixelplay-launcher/internal/download"
	"pixelplay-launcher/internal/paths"

	"golang.org/x/sys/windows/registry"
)

// Official Microsoft VC++ 2015-2022 x64 redistributable.
const RedistURL = "https://aka.ms/vs/17/release/vc_redist.x64.exe"

// Core DLLs — enough to consider the runtime usable. vcruntime140_1 is optional
// (newer builds only); requiring it caused false "missing" and setup loops when
// the installer said "another version is already installed".
var required = []string{
	"vcruntime140.dll",
	"msvcp140.dll",
}

type Progress func(string)

func metaDir() string {
	return filepath.Join(paths.DataDir(), "meta", "vcredist")
}

func ackFile() string    { return filepath.Join(metaDir(), "user_acked") }
func offeredFile() string { return filepath.Join(metaDir(), "setup_offered") }

// Present reports whether VC++ x64 is installed (registry and/or core DLLs).
func Present() bool {
	if registryInstalled() {
		return true
	}
	sys := filepath.Join(os.Getenv("SYSTEMROOT"), "System32")
	for _, name := range required {
		st, err := os.Stat(filepath.Join(sys, name))
		if err != nil || st.Size() == 0 {
			return false
		}
	}
	return true
}

func registryInstalled() bool {
	keys := []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\VisualStudio\14.0\VC\Runtimes\x64`},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\VisualStudio\14.0\VC\Runtimes\x64`},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\VisualStudio\14.0\VC\Runtimes\x64`},
	}
	for _, k := range keys {
		key, err := registry.OpenKey(k.root, k.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		installed, _, err := key.GetIntegerValue("Installed")
		key.Close()
		if err == nil && installed == 1 {
			return true
		}
	}
	return false
}

// Acknowledged is true after the user clicks "Ya lo instalé" (or we detect the
// setup was already offered and VC looks present).
func Acknowledged() bool {
	st, err := os.Stat(ackFile())
	return err == nil && st.Size() >= 0
}

// MarkAcknowledged stops the setup/popup loop.
func MarkAcknowledged() {
	_ = os.MkdirAll(metaDir(), 0o755)
	_ = os.WriteFile(ackFile(), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

func markOffered() {
	_ = os.MkdirAll(metaDir(), 0o755)
	_ = os.WriteFile(offeredFile(), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

func offeredRecently() bool {
	b, err := os.ReadFile(offeredFile())
	if err != nil {
		return false
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(b)))
	if err != nil {
		return true
	}
	return time.Since(t) < 24*time.Hour
}

// IsDLLMissingExit is Windows STATUS_DLL_NOT_FOUND (3221225781 / 0xC0000135).
func IsDLLMissingExit(code int) bool {
	return uint32(code) == 0xC0000135
}

// CachedPath returns the on-disk path used for the redistributable installer.
func CachedPath() string {
	return filepath.Join(metaDir(), "vc_redist.x64.exe")
}

// Download fetches vc_redist.x64.exe into the launcher cache.
func Download(emit Progress) (string, error) {
	dest := CachedPath()
	if st, err := os.Stat(dest); err == nil && st.Size() > 1_000_000 {
		return dest, nil
	}
	if emit != nil {
		emit("Descargando Visual C++ Redistributable…")
	}
	if err := download.File(RedistURL, dest, nil); err != nil {
		return "", fmt.Errorf("descargar VC++ Redistributable: %w", err)
	}
	return dest, nil
}

// LaunchSetup downloads (if needed) and opens the Microsoft VC++ installer UI
// with UAC elevation. Does not wait for the user to finish.
func LaunchSetup(emit Progress) (string, error) {
	dest, err := Download(emit)
	if err != nil {
		return "", err
	}
	if emit != nil {
		emit("Abriendo instalador de Visual C++…")
	}
	markOffered()
	ps := fmt.Sprintf(
		`Start-Process -FilePath '%s' -ArgumentList '/install','/norestart' -Verb RunAs`,
		strings.ReplaceAll(dest, `'`, `''`),
	)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		cmd2 := exec.Command(dest, "/install", "/norestart")
		if err2 := cmd2.Start(); err2 != nil {
			return dest, fmt.Errorf("abrir setup VC++: %w", err)
		}
	}
	return dest, nil
}

// Ensure stages VC++ DLLs next to natives. Only opens the setup if VC++ is
// truly missing and the user hasn't already acknowledged / been offered it.
func Ensure(nativesDir string, emit Progress) error {
	_ = StageInto(nativesDir)
	if Present() {
		return nil
	}
	if Acknowledged() {
		// User said it's installed (installer often reports "already installed").
		return nil
	}
	if offeredRecently() {
		// Avoid loop: setup already shown; treat as OK and let launch continue.
		MarkAcknowledged()
		return nil
	}
	if _, err := LaunchSetup(emit); err != nil {
		return err
	}
	return ErrNeedSetup
}

// RepairAfterCrash runs after exit 0xC0000135.
// If VC++ is already installed (or user acked), do NOT open the setup again —
// that causes "otra versión ya está instalada" loops.
func RepairAfterCrash(nativesDir string, emit Progress) error {
	_ = StageInto(nativesDir)
	if Present() || Acknowledged() || offeredRecently() {
		MarkAcknowledged()
		return nil
	}
	if _, err := LaunchSetup(emit); err != nil {
		return err
	}
	return ErrNeedSetup
}

// ErrNeedSetup means the Visual C++ installer was opened; user must finish it.
var ErrNeedSetup = fmt.Errorf("NEED_VCREDIST_SETUP")

// PopupMessage is the Spanish copy shown in the launcher modal.
const PopupMessage = "Minecraft necesita Visual C++ Redistributable (x64).\n\nSe abrió el instalador de Microsoft.\n\n• Si dice que ya hay otra versión instalada: cerralo y pulsá «Ya lo instalé».\n• Si no: aceptá UAC, instalá, y volvé a jugar.\n\nhttps://aka.ms/vs/17/release/vc_redist.x64.exe"

const AlreadyInstalledHint = "Visual C++ ya figura instalado en Windows. Si Minecraft sigue fallando (código 3221225781), no hace falta reinstalar VC++: borrá la carpeta meta\\natives del launcher y volvé a jugar."

// StageInto copies system VC++ DLLs into the natives folder (same directory as
// glfw.dll / lwjgl.dll) so Windows resolves them when Java loads natives.
func StageInto(nativesDir string) error {
	nativesDir = strings.TrimSpace(nativesDir)
	if nativesDir == "" {
		return nil
	}
	if err := os.MkdirAll(nativesDir, 0o755); err != nil {
		return err
	}
	sys := filepath.Join(os.Getenv("SYSTEMROOT"), "System32")
	names := append(append([]string{}, required...), "vcruntime140_1.dll", "msvcp140_1.dll", "msvcp140_2.dll", "concrt140.dll")
	for _, name := range names {
		src := filepath.Join(sys, name)
		dst := filepath.Join(nativesDir, name)
		if st, err := os.Stat(src); err != nil || st.Size() == 0 {
			continue
		}
		if st, err := os.Stat(dst); err == nil && st.Size() > 0 {
			continue
		}
		_ = copyFile(src, dst)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	cerr := out.Close()
	if err != nil {
		return err
	}
	return cerr
}

// ExplainExit maps common Windows NTSTATUS exit codes to a Spanish hint.
func ExplainExit(code int) string {
	u := uint32(code)
	switch u {
	case 0xC0000135:
		if Present() || Acknowledged() {
			return "Error de DLL al iniciar Java/Minecraft. En packs protegidos Windows ya no renombra Java (eso causaba 3221225781). Actualizá el launcher y volvé a jugar."
		}
		return PopupMessage
	case 0xC0000005:
		return "Access violation (drivers gráficos o natives corruptos). Probá borrar meta/natives y relanzar."
	case 0xC000007B:
		return "DLL/exe de arquitectura incorrecta (x86 vs x64)."
	default:
		return ""
	}
}
