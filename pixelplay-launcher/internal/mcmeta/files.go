package mcmeta

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"pixelplay-launcher/internal/download"
	"pixelplay-launcher/internal/paths"
)

func LibrariesDir() string { return filepath.Join(paths.DataDir(), "meta", "libraries") }
func AssetsDir() string    { return filepath.Join(paths.DataDir(), "meta", "assets") }
func VersionsDir() string  { return filepath.Join(paths.DataDir(), "meta", "versions") }
func NativesDir(id string) string {
	return filepath.Join(paths.DataDir(), "meta", "natives", id)
}
func ClientJar(id string) string {
	return filepath.Join(VersionsDir(), id, id+".jar")
}
func ModCacheDir() string { return filepath.Join(paths.DataDir(), "meta", "mod_cache") }

type Progress func(msg string)

func DownloadLibraries(meta *PistonMeta, extra []Library, emit Progress) ([]string, error) {
	libs := append([]Library{}, meta.Libraries...)
	libs = append(libs, extra...)
	var jars []string
	var mu sync.Mutex
	var wg sync.WaitGroup
	var fail atomic.Pointer[error]
	sem := make(chan struct{}, 8)
	n := 0
	for _, lib := range libs {
		if !RulesAllow(lib.Rules) {
			continue
		}
		if strings.Contains(lib.Name, ":natives-") && !NativeClassifierWanted(lib.Name) {
			continue
		}
		lib := lib
		wg.Add(1)
		n++
		go func() {
			defer wg.Done()
			if fail.Load() != nil {
				return
			}
			sem <- struct{}{}
			defer func() { <-sem }()
			path, err := downloadLib(lib)
			if err != nil {
				fail.CompareAndSwap(nil, &err)
				return
			}
			if path != "" {
				mu.Lock()
				jars = append(jars, path)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if errp := fail.Load(); errp != nil {
		return nil, *errp
	}
	if emit != nil {
		emit(fmt.Sprintf("Libraries: %d jars", len(jars)))
	}
	return jars, nil
}

func downloadLib(lib Library) (string, error) {
	art := artifactFor(lib)
	if art.URL == "" && art.Path == "" {
		return "", nil
	}
	rel := art.Path
	if rel == "" {
		rel = MavenPath(lib.Name)
	}
	dest := filepath.Join(LibrariesDir(), filepath.FromSlash(rel))
	if fileOK(dest, art.SHA1) {
		return dest, nil
	}
	url := art.URL
	if url == "" {
		base := strings.TrimRight(lib.URL, "/")
		if base == "" {
			base = "https://libraries.minecraft.net"
		}
		url = base + "/" + strings.TrimPrefix(rel, "/")
	}
	if err := download.File(url, dest, nil); err != nil {
		return "", fmt.Errorf("lib %s: %w", lib.Name, err)
	}
	return dest, nil
}

func artifactFor(lib Library) Artifact {
	if lib.Downloads != nil && lib.Downloads.Artifact != nil {
		return *lib.Downloads.Artifact
	}
	if lib.Downloads != nil && lib.Natives != nil {
		key, ok := lib.Natives[CurrentOSName()]
		if ok {
			if a, ok := lib.Downloads.Classifiers[key]; ok {
				return a
			}
		}
	}
	return Artifact{Path: MavenPath(lib.Name), URL: ""}
}

func fileOK(path, sha string) bool {
	st, err := os.Stat(path)
	if err != nil || st.Size() == 0 {
		return false
	}
	if sha == "" {
		return true
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	return strings.EqualFold(hex.EncodeToString(h.Sum(nil)), sha)
}

func ExtractNatives(meta *PistonMeta, extra []Library) error {
	dest := NativesDir(meta.ID)
	_ = os.MkdirAll(dest, 0o755)
	osName := CurrentOSName()
	libs := append([]Library{}, meta.Libraries...)
	libs = append(libs, extra...)
	for _, lib := range libs {
		if !RulesAllow(lib.Rules) {
			continue
		}
		if strings.Contains(lib.Name, ":natives-") && !NativeClassifierWanted(lib.Name) {
			continue
		}
		var nativeArt *Artifact
		if lib.Downloads != nil && lib.Natives != nil {
			if key, ok := lib.Natives[osName]; ok {
				if a, ok := lib.Downloads.Classifiers[key]; ok {
					aa := a
					nativeArt = &aa
				}
			}
		}
		// modern natives are separate artifacts with classifier in the name
		if nativeArt == nil && strings.Contains(lib.Name, ":natives-") {
			a := artifactFor(lib)
			nativeArt = &a
		}
		if nativeArt == nil {
			continue
		}
		rel := nativeArt.Path
		if rel == "" {
			rel = MavenPath(lib.Name)
		}
		jar := filepath.Join(LibrariesDir(), filepath.FromSlash(rel))
		if !fileOK(jar, nativeArt.SHA1) {
			url := nativeArt.URL
			if url == "" {
				url = "https://libraries.minecraft.net/" + strings.TrimPrefix(rel, "/")
			}
			if err := download.File(url, jar, nil); err != nil {
				return err
			}
		}
		if err := unzipNatives(jar, dest); err != nil {
			return fmt.Errorf("extract natives %s: %w", lib.Name, err)
		}
	}
	return nil
}

func DownloadClient(meta *PistonMeta) error {
	dest := ClientJar(meta.ID)
	if fileOK(dest, meta.Downloads.Client.SHA1) {
		return nil
	}
	return download.File(meta.Downloads.Client.URL, dest, nil)
}

func DownloadAssets(meta *PistonMeta, emit Progress) error {
	if meta.AssetIndex.URL == "" {
		return nil
	}
	idxPath := filepath.Join(AssetsDir(), "indexes", meta.AssetIndex.ID+".json")
	if !fileOK(idxPath, meta.AssetIndex.SHA1) {
		if emit != nil {
			emit("Descargando índice de assets…")
		}
		if err := download.File(meta.AssetIndex.URL, idxPath, nil); err != nil {
			return err
		}
	}
	raw, err := os.ReadFile(idxPath)
	if err != nil {
		return err
	}
	var idx struct {
		Objects map[string]struct {
			Hash string `json:"hash"`
			Size int64  `json:"size"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(raw, &idx); err != nil {
		return err
	}
	objectsDir := filepath.Join(AssetsDir(), "objects")
	type job struct {
		url, dest, hash string
	}
	var jobs []job
	for _, obj := range idx.Objects {
		hash := obj.Hash
		if len(hash) < 2 {
			continue
		}
		rel := hash[:2] + "/" + hash
		dest := filepath.Join(objectsDir, filepath.FromSlash(rel))
		if fileOK(dest, hash) {
			continue
		}
		jobs = append(jobs, job{
			url:  "https://resources.download.minecraft.net/" + rel,
			dest: dest,
			hash: hash,
		})
	}
	total := len(jobs)
	if total == 0 {
		if emit != nil {
			emit(fmt.Sprintf("Assets listos (%s)", meta.AssetIndex.ID))
		}
		return nil
	}
	if emit != nil {
		emit(fmt.Sprintf("Assets 0/%d…", total))
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	var done atomic.Int64
	var fail atomic.Pointer[error]
	var lastEmit atomic.Int64
	lastEmit.Store(time.Now().UnixNano())

	report := func() {
		if emit == nil {
			return
		}
		now := time.Now().UnixNano()
		prev := lastEmit.Load()
		if now-prev < int64(400*time.Millisecond) {
			return
		}
		if !lastEmit.CompareAndSwap(prev, now) {
			return
		}
		n := done.Load()
		emit(fmt.Sprintf("Assets %d/%d…", n, total))
	}

	for _, j := range jobs {
		j := j
		wg.Add(1)
		go func() {
			defer wg.Done()
			if fail.Load() != nil {
				return
			}
			sem <- struct{}{}
			defer func() { <-sem }()
			if fail.Load() != nil {
				return
			}
			var last error
			for attempt := 0; attempt < 3; attempt++ {
				if err := download.FileWith(download.SmallFile, j.url, j.dest, nil); err != nil {
					last = err
					time.Sleep(time.Duration(attempt+1) * 400 * time.Millisecond)
					continue
				}
				last = nil
				break
			}
			if last != nil {
				fail.CompareAndSwap(nil, &last)
				return
			}
			done.Add(1)
			report()
		}()
	}
	wg.Wait()
	if errp := fail.Load(); errp != nil {
		return fmt.Errorf("assets: %w", *errp)
	}
	if emit != nil {
		emit(fmt.Sprintf("Assets listos (%d, %s)", total, meta.AssetIndex.ID))
	}
	return nil
}
