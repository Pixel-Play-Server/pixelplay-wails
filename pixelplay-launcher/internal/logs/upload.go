package logs

import (
	"strings"

	"pixelplay-launcher/internal/accounts"
	"pixelplay-launcher/internal/httpx"
	"pixelplay-launcher/internal/version"
)

const ClientLogAPI = "https://api.pixelplay.gg/api/v1/core/client-log"

type UploadMeta struct {
	MCName           string
	UUID             string
	ProfileName      string
	Pack             string
	MinecraftVersion string
	Loader           string
	ExitCode         *int
	ProcessID        string
}

type UploadResult struct {
	OK bool   `json:"ok"`
	ID string `json:"id"`
}

func UploadClient(snap Snapshot, meta UploadMeta) (UploadResult, error) {
	body := map[string]any{
		"mc_name":           meta.MCName,
		"log":               snap.LatestLog,
		"latest_log":        snap.LatestLog,
		"crash_report":      snap.CrashReport,
		"launcher_version":  version.App,
		"profile_name":      meta.ProfileName,
		"pack":              meta.Pack,
		"minecraft_version": meta.MinecraftVersion,
		"loader":            meta.Loader,
	}
	if meta.ExitCode != nil {
		body["exit_code"] = *meta.ExitCode
	}
	headers := map[string]string{}
	if acc, err := accounts.Active(); err == nil && acc != nil {
		if meta.MCName == "" {
			body["mc_name"] = acc.Username
		}
		if tok := acc.PixelplayCredentials.Production; tok != nil && tok.Value != "" {
			headers["Authorization"] = "Bearer " + tok.Value
		}
	}
	var dest UploadResult
	if err := httpx.PostJSONHeader(ClientLogAPI, headers, body, &dest); err != nil {
		return dest, err
	}
	return dest, nil
}

func UploadForProfile(profileID string, meta UploadMeta, extra Snapshot) (UploadResult, error) {
	snap := extra
	if strings.TrimSpace(snap.LatestLog) == "" && strings.TrimSpace(snap.CrashReport) == "" {
		if cached := LoadSnapshot(profileID); cached.LatestLog != "" || cached.CrashReport != "" {
			snap = cached
		} else if combined, err := LatestForProfile(profileID); err == nil {
			snap.LatestLog = combined
		}
	}
	return UploadClient(snap, meta)
}
