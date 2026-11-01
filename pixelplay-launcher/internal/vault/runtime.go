package vault

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// NewRuntimeDir creates an obscure temp folder (dot + random hex) for decrypted play.
func NewRuntimeDir() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	name := "." + hex.EncodeToString(b[:])
	dir := filepath.Join(os.TempDir(), name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// StageJava copies the JVM launcher into the runtime so `ps` shows a random path/name.
func StageJava(runtimeDir, javaBin string) (string, error) {
	if javaBin == "" || runtimeDir == "" {
		return javaBin, nil
	}
	// Windows: java.exe needs sibling DLLs under bin/ (java.dll, server/jvm.dll, …).
	// Copying only the exe and symlinking lib/ fails (symlinks often need admin) and
	// yields STATUS_DLL_NOT_FOUND (exit 3221225781 / 0xC0000135). Keep the real path.
	if runtime.GOOS == "windows" {
		return javaBin, nil
	}
	abs, err := filepath.Abs(javaBin)
	if err != nil {
		abs = javaBin
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	home := filepath.Dir(filepath.Dir(abs))
	if filepath.Base(filepath.Dir(abs)) != "bin" {
		home = filepath.Dir(abs)
	}
	var tag [4]byte
	_, _ = rand.Read(tag[:])
	destHome := filepath.Join(runtimeDir, "."+hex.EncodeToString(tag[:]))
	if err := os.MkdirAll(filepath.Join(destHome, "bin"), 0o700); err != nil {
		return "", err
	}
	var exe [3]byte
	_, _ = rand.Read(exe[:])
	exeName := hex.EncodeToString(exe[:])
	staged := filepath.Join(destHome, "bin", exeName)
	if err := copyFile(abs, staged, 0o755); err != nil {
		return "", fmt.Errorf("copiar java: %w", err)
	}
	for _, n := range []string{"lib", "conf", "jmods", "legal", "release"} {
		src := filepath.Join(home, n)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		_ = os.Symlink(src, filepath.Join(destHome, n))
	}
	return staged, nil
}

// ShredDir overwrites files then deletes the runtime tree. Never follows symlinks
// (the staged JRE lib/ is a symlink into the real Java home).
func ShredDir(dir string) error {
	if dir == "" {
		return nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	tmp, err := filepath.Abs(os.TempDir())
	if err != nil {
		return err
	}
	if filepath.Dir(abs) != tmp || !strings.HasPrefix(filepath.Base(abs), ".") {
		return fmt.Errorf("refusado shred fuera de temp: %s", abs)
	}
	_ = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			_ = os.Remove(path)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			return nil
		}
		buf := make([]byte, 32*1024)
		left := info.Size()
		for left > 0 {
			n := len(buf)
			if int64(n) > left {
				n = int(left)
			}
			_, _ = f.Write(buf[:n])
			left -= int64(n)
		}
		_ = f.Close()
		return os.Remove(path)
	})
	return os.RemoveAll(abs)
}

// MigrateFrom copies a legacy plaintext profile into the encrypted vault.
func MigrateFrom(profileDir, vaultRoot, packID string) error {
	if profileDir == "" || vaultRoot == "" || packID == "" {
		return nil
	}
	if filepath.Clean(profileDir) == filepath.Clean(vaultRoot) {
		return nil
	}
	if err := SealFrom(profileDir, vaultRoot, packID); err != nil {
		return err
	}
	for _, folder := range sealedTrees {
		_ = os.RemoveAll(filepath.Join(profileDir, folder))
	}
	for _, name := range rootPersist {
		_ = os.Remove(filepath.Join(profileDir, name))
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
