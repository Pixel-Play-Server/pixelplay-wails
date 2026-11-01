package packs

import (
	"strings"

	"pixelplay-launcher/internal/modpackupdate"
	"pixelplay.gg/ppack"
)

// ApplyRemoteUpdate checks the pack updater URL and installs a newer .ppack.
// Packs without updater are skipped. emit may be nil.
func ApplyRemoteUpdate(packID string, emit func(string, any)) (from, to string, applied bool, err error) {
	up, cur, err := UpdaterFor(packID)
	if err != nil || up == nil || strings.TrimSpace(up.URL) == "" {
		return "", "", false, nil
	}
	man, newer, err := modpackupdate.Check(cur, up.URL)
	if err != nil {
		return cur, "", false, err
	}
	if !newer || man == nil {
		return cur, cur, false, nil
	}
	if emit != nil {
		emit("modpack_update_available", map[string]any{
			"pack_id":     packID,
			"displayName": DisplayNameOf(packID, packID),
			"from":        cur,
			"to":          man.Version,
			"url":         man.URL,
			"notes":       man.Notes,
		})
		emit("modpack_update_status", map[string]any{
			"pack_id": packID,
			"status":  "downloading",
			"current": cur,
			"to":      man.Version,
		})
	}
	path, err := modpackupdate.Download(packID, man)
	if err != nil {
		return cur, man.Version, false, err
	}
	imported, err := ppack.ReadManifestFromZip(path)
	if err != nil {
		return cur, man.Version, false, err
	}
	if imported.ID != "" && imported.ID != packID {
		return cur, cur, false, nil
	}
	if _, err := ImportPPackFile(path); err != nil {
		return cur, man.Version, false, err
	}
	_ = ApplyOverridesToPackProfiles(packID)
	if emit != nil {
		emit("modpack_updated", map[string]any{
			"pack_id":     packID,
			"displayName": DisplayNameOf(packID, packID),
			"from":        cur,
			"to":          man.Version,
			"path":        path,
		})
	}
	return cur, man.Version, true, nil
}
