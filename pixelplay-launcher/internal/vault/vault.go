package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	Ext    = ".ppv"
	magic  = "PPVAULT1"
	noncel = 12
)

var sealedTrees = []string{"mods", "config", "kubejs", "defaultconfigs", "resourcepacks", "shaderpacks", "saves"}

var rootPersist = []string{
	"options.txt", "optionsof.txt", "optionsshaders.txt",
	"servers.dat", "servers.dat_old",
	"usernamecache.json", "usercache.json", "hotbar.nbt",
}

func masterKey() []byte {
	x := []byte{
		0x4e, 0x91, 0x2c, 0x77, 0x08, 0xd4, 0xb1, 0x3f,
		0x6a, 0xe2, 0x15, 0x9c, 0x43, 0x80, 0xfa, 0x27,
		0x5d, 0x0b, 0xc8, 0x64, 0x19, 0xae, 0x32, 0xf7,
		0x8c, 0x51, 0xd0, 0x06, 0xbb, 0x3a, 0x74, 0xe9,
	}
	for i := range x {
		x[i] ^= 0xa5 ^ byte(i*31+7)
	}
	return x
}

func keyFor(packID string) []byte {
	h := sha256.New()
	h.Write(masterKey())
	h.Write([]byte{0})
	h.Write([]byte(packID))
	h.Write([]byte{0, 'v', '1'})
	return h.Sum(nil)
}

func objectName(packID, rel string) string {
	rel = filepath.ToSlash(rel)
	sum := sha256.Sum256([]byte(packID + "\x00" + rel))
	return hex.EncodeToString(sum[:16]) + Ext
}

// ObjectName returns the hashed blob filename for a sealed relative path.
func ObjectName(packID, rel string) string {
	return objectName(packID, rel)
}

func gcmFor(packID string) (cipher.AEAD, error) {
	block, err := aes.NewCipher(keyFor(packID))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func LooksSealed(b []byte) bool {
	return len(b) >= 8 && string(b[:8]) == magic
}

// Put writes an encrypted blob named after a hash of packID+rel into dir.
func Put(dir, rel, packID string, plaintext []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	blob, err := wrap(packID, rel, plaintext)
	if err != nil {
		return err
	}
	dest := filepath.Join(dir, objectName(packID, rel))
	if err := os.WriteFile(dest, blob, 0o600); err != nil {
		return err
	}
	orig := filepath.Join(dir, filepath.FromSlash(rel))
	if filepath.Clean(orig) != filepath.Clean(dest) {
		_ = os.Remove(orig)
	}
	return nil
}

// Remove deletes the plaintext file and its sealed blob.
func Remove(dir, rel, packID string) {
	_ = os.Remove(filepath.Join(dir, filepath.FromSlash(rel)))
	_ = os.Remove(filepath.Join(dir, objectName(packID, rel)))
}

func wrap(packID, rel string, plaintext []byte) ([]byte, error) {
	rel = filepath.ToSlash(rel)
	aead, err := gcmFor(packID)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, noncel)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	name := []byte(rel)
	if len(name) > 0xffff {
		return nil, fmt.Errorf("nombre demasiado largo")
	}
	payload := make([]byte, 2+len(name)+len(plaintext))
	binary.BigEndian.PutUint16(payload[:2], uint16(len(name)))
	copy(payload[2:], name)
	copy(payload[2+len(name):], plaintext)
	ct := aead.Seal(nil, nonce, payload, []byte(magic))
	out := make([]byte, 0, 8+noncel+len(ct))
	out = append(out, magic...)
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

func unwrap(packID string, blob []byte) (rel string, plain []byte, err error) {
	if !LooksSealed(blob) {
		return "", nil, fmt.Errorf("no es un blob Pixel Play")
	}
	if len(blob) < 8+noncel+16 {
		return "", nil, fmt.Errorf("blob truncado")
	}
	aead, err := gcmFor(packID)
	if err != nil {
		return "", nil, err
	}
	nonce := blob[8 : 8+noncel]
	payload, err := aead.Open(nil, nonce, blob[8+noncel:], []byte(magic))
	if err != nil {
		return "", nil, fmt.Errorf("no se pudo abrir (pack/clave distinta)")
	}
	if len(payload) < 2 {
		return "", nil, fmt.Errorf("payload vacío")
	}
	n := int(binary.BigEndian.Uint16(payload[:2]))
	if 2+n > len(payload) {
		return "", nil, fmt.Errorf("nombre truncado")
	}
	rel = string(payload[2 : 2+n])
	plain = payload[2+n:]
	return rel, plain, nil
}

// SealTree encrypts every regular file under root into hashed .ppv blobs at root.
func SealTree(root, packID string) error {
	if root == "" || packID == "" {
		return nil
	}
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		return nil
	}
	var files []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), Ext) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	for _, path := range files {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if LooksSealed(data) {
			continue
		}
		if err := Put(root, rel, packID, data); err != nil {
			return err
		}
		if filepath.Clean(path) != filepath.Clean(filepath.Join(root, objectName(packID, rel))) {
			_ = os.Remove(path)
		}
	}
	return pruneEmpty(root)
}

// UnsealTree decrypts hashed .ppv blobs back to their original relative paths.
func UnsealTree(root, packID string) error {
	return unsealDir(root, root, packID)
}

// UnsealTo decrypts blobs from srcRoot into dstRoot (used for the temp runtime).
func UnsealTo(srcRoot, dstRoot, packID string) error {
	if srcRoot == "" || dstRoot == "" || packID == "" {
		return nil
	}
	if err := os.MkdirAll(dstRoot, 0o700); err != nil {
		return err
	}
	for _, folder := range sealedTrees {
		if err := unsealDir(filepath.Join(srcRoot, folder), filepath.Join(dstRoot, folder), packID); err != nil {
			return err
		}
	}
	return unsealDir(srcRoot, dstRoot, packID)
}

func unsealDir(srcRoot, dstRoot, packID string) error {
	if srcRoot == "" || dstRoot == "" || packID == "" {
		return nil
	}
	entries, err := os.ReadDir(srcRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), Ext) {
			continue
		}
		p := filepath.Join(srcRoot, e.Name())
		blob, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, plain, err := unwrap(packID, blob)
		if err != nil {
			continue
		}
		dest := filepath.Join(dstRoot, filepath.FromSlash(rel))
		if !within(dstRoot, dest) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(dest, plain, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// WipePlaintext deletes decrypted files, leaving only .ppv blobs.
func WipePlaintext(root, packID string) error {
	if root == "" {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), Ext) {
			return nil
		}
		return os.Remove(path)
	})
}

func SealGameDir(gameDir, packID string) error {
	return SealFrom(gameDir, gameDir, packID)
}

func UnsealGameDir(gameDir, packID string) error {
	return UnsealTo(gameDir, gameDir, packID)
}

// SealFrom encrypts plaintext files from plainRoot into vaultRoot (hashed .ppv).
func SealFrom(plainRoot, vaultRoot, packID string) error {
	if plainRoot == "" || vaultRoot == "" || packID == "" {
		return nil
	}
	for _, folder := range sealedTrees {
		src := filepath.Join(plainRoot, folder)
		dst := filepath.Join(vaultRoot, folder)
		if err := sealDirFrom(src, dst, packID); err != nil {
			return err
		}
	}
	for _, name := range rootPersist {
		p := filepath.Join(plainRoot, name)
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if LooksSealed(b) {
			continue
		}
		if err := Put(vaultRoot, name, packID, b); err != nil {
			return err
		}
		if filepath.Clean(plainRoot) != filepath.Clean(vaultRoot) {
			continue
		}
		_ = os.Remove(p)
	}
	if filepath.Clean(plainRoot) == filepath.Clean(vaultRoot) {
		for _, folder := range sealedTrees {
			p := filepath.Join(plainRoot, folder)
			_ = WipePlaintext(p, packID)
			_ = pruneEmpty(p)
		}
	}
	return nil
}

func sealDirFrom(src, dst, packID string) error {
	st, err := os.Stat(src)
	if err != nil || !st.IsDir() {
		return nil
	}
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), Ext) {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if LooksSealed(data) {
			return nil
		}
		return Put(dst, rel, packID, data)
	})
}

func within(root, dest string) bool {
	rel, err := filepath.Rel(root, dest)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func pruneEmpty(root string) error {
	var dirs []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && path != root {
			dirs = append(dirs, path)
		}
		return err
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		_ = os.Remove(dirs[i])
	}
	return nil
}
