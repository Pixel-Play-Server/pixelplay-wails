package profiles

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"pixelplay-launcher/internal/paths"
)

// Summary is the lightweight profile view for the UI list.
type Summary struct {
	ID                      string  `json:"id"`
	Name                    string  `json:"name"`
	Path                    string  `json:"path"`
	GameVersion             string  `json:"game_version"`
	Loader                  string  `json:"loader"`
	LoaderVersion           *string `json:"loader_version"`
	Group                   *string `json:"group"`
	SelectedPixelplayPackID *string `json:"selected_pixelplay_pack_id"`
	State                   string  `json:"state"`
	Description             *string `json:"description"`
	IsStandardVersion       bool    `json:"is_standard_version"`
}

type rawProfile struct {
	ID                      string  `json:"id"`
	Name                    string  `json:"name"`
	Path                    string  `json:"path"`
	GameVersion             string  `json:"game_version"`
	Loader                  string  `json:"loader"`
	LoaderVersion           *string `json:"loader_version"`
	Group                   *string `json:"group"`
	SelectedPixelplayPackID *string `json:"selected_pixelplay_pack_id"`
	State                   string  `json:"state"`
	Description             *string `json:"description"`
	IsStandardVersion       bool    `json:"is_standard_version"`
}

// RawList returns the full profiles.json array (exact UI Profile shape).
func RawList() (json.RawMessage, error) {
	data, err := os.ReadFile(paths.ProfilesFile())
	if err != nil {
		return nil, fmt.Errorf("leer profiles.json: %w", err)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("profiles.json inválido")
	}
	return json.RawMessage(data), nil
}

// RawGet returns one full profile object by id.
func RawGet(id string) (json.RawMessage, error) {
	data, err := os.ReadFile(paths.ProfilesFile())
	if err != nil {
		return nil, fmt.Errorf("leer profiles.json: %w", err)
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err != nil {
		return nil, fmt.Errorf("parse profiles.json: %w", err)
	}
	for _, item := range arr {
		var meta struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(item, &meta); err != nil {
			continue
		}
		if meta.ID == id {
			return item, nil
		}
	}
	return nil, fmt.Errorf("perfil no encontrado: %s", id)
}

// List reads profiles.json from the shared data directory.
func List() ([]Summary, error) {
	data, err := os.ReadFile(paths.ProfilesFile())
	if err != nil {
		return nil, fmt.Errorf("leer profiles.json: %w", err)
	}
	var raw []rawProfile
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse profiles.json: %w", err)
	}
	out := make([]Summary, 0, len(raw))
	for _, p := range raw {
		out = append(out, Summary{
			ID:                      p.ID,
			Name:                    p.Name,
			Path:                    p.Path,
			GameVersion:             p.GameVersion,
			Loader:                  p.Loader,
			LoaderVersion:           p.LoaderVersion,
			Group:                   p.Group,
			SelectedPixelplayPackID: p.SelectedPixelplayPackID,
			State:                   p.State,
			Description:             p.Description,
			IsStandardVersion:       p.IsStandardVersion,
		})
	}
	return out, nil
}

// Search filters profiles by name/group substring (case-insensitive).
func Search(query string) ([]Summary, error) {
	all, err := List()
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return all, nil
	}
	out := make([]Summary, 0)
	for _, p := range all {
		if strings.Contains(strings.ToLower(p.Name), q) {
			out = append(out, p)
			continue
		}
		if p.Group != nil && strings.Contains(strings.ToLower(*p.Group), q) {
			out = append(out, p)
		}
	}
	return out, nil
}

// Get returns one profile summary by id.
func Get(id string) (*Summary, error) {
	all, err := List()
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].ID == id {
			return &all[i], nil
		}
	}
	return nil, fmt.Errorf("perfil no encontrado: %s", id)
}

// OpenFolder returns the absolute path of the profile directory.
func OpenFolder(id string) (string, error) {
	p, err := Get(id)
	if err != nil {
		return "", err
	}
	dir := paths.ProfileDir(p.Path)
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("carpeta del perfil: %w", err)
	}
	return dir, nil
}

func loadAll() ([]map[string]any, error) {
	data, err := os.ReadFile(paths.ProfilesFile())
	if err != nil {
		if os.IsNotExist(err) {
			return []map[string]any{}, nil
		}
		return nil, err
	}
	var arr []map[string]any
	if err := json.Unmarshal(data, &arr); err != nil {
		return nil, err
	}
	return arr, nil
}

func saveAll(arr []map[string]any) error {
	b, err := json.MarshalIndent(arr, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(paths.ProfilesFile(), b, 0o644)
}

func slugPath(name string) string {
	s := strings.TrimSpace(name)
	if s == "" {
		s = "profile"
	}
	return s
}

func defaultSettings() map[string]any {
	return map[string]any{
		"java_path":                    nil,
		"use_custom_java_path":         false,
		"use_overwrite_loader_version": false,
		"overwrite_loader_version":     nil,
		"memory":                       map[string]any{"min": 2048, "max": 4096},
		"resolution":                   nil,
		"fullscreen":                   false,
		"extra_game_args":              []any{},
		"custom_jvm_args":              nil,
		"quick_play_path":              nil,
		"hooks": map[string]any{
			"pre_launch": nil,
			"wrapper":    nil,
			"post_exit":  nil,
		},
	}
}

func Create(params map[string]any) (string, error) {
	arr, err := loadAll()
	if err != nil {
		return "", err
	}
	id := uuid.NewString()
	name, _ := params["name"].(string)
	if name == "" {
		name = "Nuevo perfil"
	}
	gv, _ := params["game_version"].(string)
	if gv == "" {
		gv = "1.21.1"
	}
	loader, _ := params["loader"].(string)
	if loader == "" {
		loader = "vanilla"
	}
	var loaderVer any
	if v, ok := params["loader_version"]; ok {
		loaderVer = v
	}
	var pack any
	if v, ok := params["selected_pixelplay_pack_id"]; ok {
		pack = v
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	p := map[string]any{
		"id":                               id,
		"name":                             name,
		"path":                             slugPath(name),
		"game_version":                     gv,
		"loader":                           strings.ToLower(loader),
		"loader_version":                   loaderVer,
		"created":                          now,
		"last_played":                      nil,
		"settings":                         defaultSettings(),
		"state":                            "not_installed",
		"mods":                             []any{},
		"selected_pixelplay_pack_id":       pack,
		"disabled_pixelplay_mods_detailed": []any{},
		"source_standard_profile_id":       nil,
		"group":                            nil,
		"use_shared_minecraft_folder":      false,
		"is_standard_version":              false,
		"description":                      nil,
		"banner":                           nil,
		"background":                       nil,
		"pixelplay_information":            nil,
		"modpack_info":                     nil,
	}
	arr = append(arr, p)
	if err := saveAll(arr); err != nil {
		return "", err
	}
	dir := paths.ProfileDir(p["path"].(string))
	_ = os.MkdirAll(dir, 0o755)
	return id, nil
}

func Update(id string, params map[string]any) error {
	arr, err := loadAll()
	if err != nil {
		return err
	}
	found := false
	for i, p := range arr {
		pid, _ := p["id"].(string)
		if pid != id {
			continue
		}
		found = true
		for _, k := range []string{"name", "game_version", "loader", "loader_version", "group", "description", "settings", "selected_pixelplay_pack_id", "use_shared_minecraft_folder", "pixelplay_information"} {
			if v, ok := params[k]; ok && v != nil {
				arr[i][k] = v
			}
		}
		if v, ok := params["clear_group"].(bool); ok && v {
			arr[i]["group"] = nil
		}
		if v, ok := params["clear_selected_pixelplay_pack"].(bool); ok && v {
			arr[i]["selected_pixelplay_pack_id"] = nil
		}
		break
	}
	if !found {
		return fmt.Errorf("perfil no encontrado: %s", id)
	}
	return saveAll(arr)
}

func Delete(id string) error {
	arr, err := loadAll()
	if err != nil {
		return err
	}
	out := arr[:0]
	for _, p := range arr {
		pid, _ := p["id"].(string)
		if pid != id {
			out = append(out, p)
		}
	}
	return saveAll(out)
}

func Copy(sourceID, newName string) (string, error) {
	raw, err := RawGet(sourceID)
	if err != nil {
		return "", err
	}
	var p map[string]any
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", err
	}
	id := uuid.NewString()
	p["id"] = id
	if newName != "" {
		p["name"] = newName
		p["path"] = slugPath(newName)
	}
	p["created"] = time.Now().UTC().Format(time.RFC3339Nano)
	p["last_played"] = nil
	arr, err := loadAll()
	if err != nil {
		return "", err
	}
	arr = append(arr, p)
	if err := saveAll(arr); err != nil {
		return "", err
	}
	return id, nil
}

func SetPixelplayModStatus(profileID, packID, modID, gameVersion, loader string, disabled bool) error {
	arr, err := loadAll()
	if err != nil {
		return err
	}
	identMatch := func(m map[string]any) bool {
		return fmt.Sprint(m["pack_id"]) == packID &&
			fmt.Sprint(m["mod_id"]) == modID &&
			fmt.Sprint(m["game_version"]) == gameVersion &&
			strings.EqualFold(fmt.Sprint(m["loader"]), loader)
	}
	for i, p := range arr {
		pid, _ := p["id"].(string)
		if pid != profileID {
			continue
		}
		raw, _ := p["disabled_pixelplay_mods_detailed"].([]any)
		out := make([]any, 0, len(raw)+1)
		found := false
		for _, item := range raw {
			m, _ := item.(map[string]any)
			if m != nil && identMatch(m) {
				found = true
				if disabled {
					out = append(out, item)
				}
				continue
			}
			out = append(out, item)
		}
		if disabled && !found {
			out = append(out, map[string]any{
				"pack_id":      packID,
				"mod_id":       modID,
				"game_version": gameVersion,
				"loader":       loader,
			})
		}
		arr[i]["disabled_pixelplay_mods_detailed"] = out
		return saveAll(arr)
	}
	return fmt.Errorf("perfil no encontrado: %s", profileID)
}

func SetModEnabled(profileID, modID string, enabled bool) error {
	arr, err := loadAll()
	if err != nil {
		return err
	}
	for i, p := range arr {
		pid, _ := p["id"].(string)
		if pid != profileID {
			continue
		}
		mods, _ := p["mods"].([]any)
		for j, item := range mods {
			m, _ := item.(map[string]any)
			if m == nil {
				continue
			}
			if fmt.Sprint(m["id"]) != modID {
				continue
			}
			m["enabled"] = enabled
			mods[j] = m
			arr[i]["mods"] = mods
			return saveAll(arr)
		}
		return fmt.Errorf("mod no encontrado: %s", modID)
	}
	return fmt.Errorf("perfil no encontrado: %s", profileID)
}

func TouchLastPlayed(id string) {
	arr, err := loadAll()
	if err != nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i, p := range arr {
		if pid, _ := p["id"].(string); pid == id {
			arr[i]["last_played"] = now
			arr[i]["state"] = "installed"
			_ = saveAll(arr)
			return
		}
	}
}
