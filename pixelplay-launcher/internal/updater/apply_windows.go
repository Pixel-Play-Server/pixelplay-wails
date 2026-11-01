//go:build windows

package updater

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	windowsDetachedProcess       = 0x00000008
	windowsCreateNewProcessGroup = 0x00000200
	windowsCreateBreakawayFromJob = 0x01000000
)

// ApplyAndRelaunch waits for this process to exit, drop-in replaces the exe, then relaunches.
// NSIS is only for first install from the web — in-app updates always use the raw .exe.
func ApplyAndRelaunch(newPath string) error {
	if newPath == "" {
		return fmt.Errorf("sin binario nuevo")
	}
	newPath, err := filepath.Abs(newPath)
	if err != nil {
		return err
	}
	if looksLikeInstaller(newPath) {
		return fmt.Errorf("el update drop-in recibió un instalador NSIS (%s); se esperaba pixelplay-launcher.exe", filepath.Base(newPath))
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return err
	}
	// Prefer the stable install path so Updates always land where shortcuts point.
	stable := stableInstallExe()
	targets := uniquePaths(exe, stable)

	dir := filepath.Join(filepath.Dir(exe), "..")
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		dir = filepath.Join(d, "Programs", "PixelPlay")
	}
	_ = os.MkdirAll(dir, 0o755)
	logPath := filepath.Join(filepath.Dir(newPath), "apply.log")
	ps1Path := filepath.Join(filepath.Dir(newPath), "pixelplay-apply-update.ps1")
	pid := os.Getpid()

	targetList := ""
	for i, t := range targets {
		if i > 0 {
			targetList += ", "
		}
		targetList += psLit(t)
	}

	script := fmt.Sprintf(`$ErrorActionPreference = 'Continue'
$log = %s
$new = %s
$targets = @(%s)
$waitPid = %d
$relaunch = $targets[0]
function W($m) { try { Add-Content -LiteralPath $log -Value ("{0:yyyy-MM-dd HH:mm:ss} {1}" -f (Get-Date), $m) } catch {} }
W ("apply drop-in start pid=" + $waitPid + " new=" + $new)
W ("targets=" + ($targets -join ' | '))
if ($new -match '(?i)(installer|setup)') {
  W 'REFUSING installer artifact — drop-in requires pixelplay-launcher.exe'
  exit 2
}
for ($i = 0; $i -lt 180; $i++) {
  $p = Get-Process -Id $waitPid -ErrorAction SilentlyContinue
  if (-not $p) { break }
  Start-Sleep -Milliseconds 500
}
Start-Sleep -Milliseconds 600
# Stop other launcher instances that could lock the file
Get-Process -ErrorAction SilentlyContinue |
  Where-Object { $_.ProcessName -match '^(pixelplay-launcher|Pixel Play)$' -and $_.Id -ne $waitPid } |
  ForEach-Object {
    try { Stop-Process -Id $_.Id -Force -ErrorAction SilentlyContinue; W ("killed pid=" + $_.Id) } catch {}
  }
Start-Sleep -Milliseconds 400
$copied = $false
foreach ($exe in $targets) {
  $destDir = Split-Path -Parent $exe
  if ($destDir -and -not (Test-Path -LiteralPath $destDir)) {
    New-Item -ItemType Directory -Force -Path $destDir | Out-Null
  }
  for ($r = 1; $r -le 40; $r++) {
    try {
      Copy-Item -LiteralPath $new -Destination $exe -Force
      W ("copy ok -> " + $exe + " attempt=" + $r)
      $copied = $true
      $relaunch = $exe
      break
    } catch {
      W ("copy retry " + $r + " -> " + $exe + ": " + $_.Exception.Message)
      Start-Sleep -Milliseconds 500
    }
  }
}
if (-not $copied) {
  W 'copy failed all targets'
  exit 1
}
try {
  Start-Process -FilePath $relaunch -WorkingDirectory (Split-Path -Parent $relaunch)
  W ('relaunch ok -> ' + $relaunch)
} catch {
  W ('relaunch failed: ' + $_.Exception.Message)
  exit 1
}
Remove-Item -LiteralPath $MyInvocation.MyCommand.Path -Force -ErrorAction SilentlyContinue
`, psLit(logPath), psLit(newPath), targetList, pid)

	if err := os.WriteFile(ps1Path, []byte(script), 0o755); err != nil {
		return err
	}

	// Detach via cmd `start` so the helper survives when the launcher exits.
	cmd := exec.Command("cmd.exe", "/C", "start", "", "/MIN",
		"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-File", ps1Path)
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windowsDetachedProcess | windowsCreateNewProcessGroup | windowsCreateBreakawayFromJob,
	}
	if err := cmd.Start(); err != nil {
		// Fallback: direct powershell
		cmd2 := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-File", ps1Path)
		cmd2.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: windowsDetachedProcess | windowsCreateNewProcessGroup | windowsCreateBreakawayFromJob,
		}
		return cmd2.Start()
	}
	return nil
}

func looksLikeInstaller(path string) bool {
	b := strings.ToLower(filepath.Base(path))
	return strings.Contains(b, "installer") || strings.Contains(b, "setup") || strings.HasSuffix(b, "-setup.exe")
}

func stableInstallExe() string {
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		return filepath.Join(local, "Programs", "PixelPlay", "pixelplay-launcher.exe")
	}
	return ""
}

func uniquePaths(paths ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		key := strings.ToLower(filepath.Clean(p))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
}

func psLit(s string) string {
	out := "'"
	for _, r := range s {
		if r == '\'' {
			out += "''"
		} else {
			out += string(r)
		}
	}
	return out + "'"
}
