package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"pixelplay-launcher/internal/accounts"
	"pixelplay-launcher/internal/auth"
	"pixelplay-launcher/internal/capes"
	"pixelplay-launcher/internal/capetemplates"
	"pixelplay-launcher/internal/config"
	"pixelplay-launcher/internal/content"
	"pixelplay-launcher/internal/curseforge"
	"pixelplay-launcher/internal/discordrpc"
	"pixelplay-launcher/internal/download"
	"pixelplay-launcher/internal/featured"
	"pixelplay-launcher/internal/horizon"
	"pixelplay-launcher/internal/httpx"
	"pixelplay-launcher/internal/java"
	"pixelplay-launcher/internal/launch"
	"pixelplay-launcher/internal/logs"
	"pixelplay-launcher/internal/mcmeta"
	"pixelplay-launcher/internal/modpackupdate"
	"pixelplay-launcher/internal/modrinth"
	"pixelplay-launcher/internal/mrpack"
	"pixelplay-launcher/internal/notify"
	"pixelplay-launcher/internal/osopen"
	"pixelplay-launcher/internal/packs"
	"pixelplay-launcher/internal/paths"
	"pixelplay-launcher/internal/process"
	"pixelplay-launcher/internal/profiles"
	"pixelplay-launcher/internal/protect"
	"pixelplay-launcher/internal/skins"
	"pixelplay-launcher/internal/unified"
	"pixelplay-launcher/internal/updater"
	"pixelplay-launcher/internal/vanillacapes"
	"pixelplay-launcher/internal/vcredist"
	"pixelplay-launcher/internal/version"
	"pixelplay-launcher/internal/worlds"
)

// Runtime is the app-facing bridge (events + discord).
type Runtime struct {
	Emit            func(event string, payload any)
	Discord         *discordrpc.Manager
	OnAuthorBlocked func(action string)
	OpenURL         func(url string)
	PickFile        func(title, filter string) (string, error)
	PickFiles       func(title, filter string) ([]string, error)
	PickDir         func(title string) (string, error)
	SaveFile        func(title, suggested, filter string) (string, error)
}

// Dispatch routes Tauri-compatible command names to Go handlers.
func Dispatch(rt *Runtime, cmd string, argsJSON string) (any, error) {
	args := map[string]json.RawMessage{}
	if strings.TrimSpace(argsJSON) != "" && argsJSON != "null" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return nil, fmt.Errorf("args: %w", err)
		}
	}
	emit := func(event string, payload any) {
		if rt != nil && rt.Emit != nil {
			rt.Emit(event, payload)
		}
	}

	switch cmd {
	case "set_active_account", "remove_account", "login_pixel_account", "reserve_pixel_account":
		defer reconnectSocial()
	}

	switch cmd {
	case "list_profiles":
		return profiles.RawList()
	case "get_profile":
		id, err := argString(args, "id")
		if err != nil {
			return nil, err
		}
		return profiles.RawGet(id)
	case "search_profiles":
		q, _ := argString(args, "query")
		list, err := profiles.Search(q)
		if err != nil {
			return nil, err
		}
		raw, err := profiles.RawList()
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(q) == "" {
			return raw, nil
		}
		var all []json.RawMessage
		if err := json.Unmarshal(raw, &all); err != nil {
			return nil, err
		}
		ids := map[string]struct{}{}
		for _, s := range list {
			ids[s.ID] = struct{}{}
		}
		out := make([]json.RawMessage, 0, len(list))
		for _, item := range all {
			var meta struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(item, &meta)
			if _, ok := ids[meta.ID]; ok {
				out = append(out, item)
			}
		}
		return out, nil

	case "list_horizon_registry_mods":
		return horizon.RegistryMods(context.Background())
	case "install_horizon_registry_mod":
		id, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		mod, err := argString(args, "modId", "mod_id")
		if err != nil {
			return nil, err
		}
		raw, err := profiles.RawGet(id)
		if err != nil {
			return nil, err
		}
		var p struct {
			Path    string `json:"path"`
			Horizon bool   `json:"horizon_runtime"`
		}
		if err = json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		if !p.Horizon || p.Path == "" || !filepath.IsLocal(p.Path) {
			return nil, fmt.Errorf("perfil Horizon inválido")
		}
		return horizon.InstallRegistryMod(context.Background(), paths.ProfileDir(p.Path), mod)
	case "list_profile_screenshots":
		id, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		p, err := profiles.Get(id)
		if err != nil {
			return nil, err
		}
		dir := filepath.Join(paths.ProfileDir(p.Path), "screenshots")
		items, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			return []map[string]any{}, nil
		}
		if err != nil {
			return nil, err
		}
		out := []map[string]any{}
		for _, item := range items {
			if !item.Type().IsRegular() {
				continue
			}
			name := item.Name()
			low := strings.ToLower(name)
			if !strings.HasSuffix(low, ".png") && !strings.HasSuffix(low, ".jpg") && !strings.HasSuffix(low, ".jpeg") {
				continue
			}
			stat, e := item.Info()
			if e != nil {
				continue
			}
			out = append(out, map[string]any{"filename": name, "path": filepath.Join(dir, name), "modified": stat.ModTime().UTC().Format(time.RFC3339)})
		}
		return out, nil
	case "install_horizon_mod":
		id, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		source, err := argString(args, "source")
		if err != nil {
			return nil, err
		}
		raw, err := profiles.RawGet(id)
		if err != nil {
			return nil, err
		}
		var p struct {
			Path    string `json:"path"`
			Horizon bool   `json:"horizon_runtime"`
		}
		if err = json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		if !p.Horizon || p.Path == "" || filepath.IsAbs(p.Path) || filepath.Clean(p.Path) == ".." || strings.HasPrefix(filepath.Clean(p.Path), ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("se requiere un perfil Horizon válido")
		}
		return horizon.InstallMod(paths.ProfileDir(p.Path), source)
	case "create_profile":
		params := map[string]any{}
		_ = json.Unmarshal([]byte(argsJSON), &params)
		if raw, ok := args["params"]; ok {
			_ = json.Unmarshal(raw, &params)
		}
		return profiles.Create(params)
	case "update_profile":
		id, err := argString(args, "id")
		if err != nil {
			return nil, err
		}
		params := map[string]any{}
		if raw, ok := args["params"]; ok {
			_ = json.Unmarshal(raw, &params)
		}
		return nil, profiles.Update(id, params)
	case "delete_profile":
		id, err := argString(args, "id")
		if err != nil {
			return nil, err
		}
		return nil, profiles.Delete(id)
	case "copy_profile":
		params := args
		if raw, ok := args["params"]; ok {
			_ = json.Unmarshal(raw, &params)
		}
		src, err := argString(params, "source_profile_id", "sourceProfileId")
		if err != nil {
			return nil, err
		}
		name, _ := argString(params, "new_profile_name", "newProfileName")
		return profiles.Copy(src, name)
	case "repair_profile":
		id, err := argString(args, "id")
		if err != nil {
			return nil, err
		}
		emit("state_event", map[string]any{
			"event_type": "profile_update", "target_id": id, "message": "Repair: reinstalá lanzando Play",
		})
		return nil, nil

	case "get_pixelplay_packs", "get_pixelplay_packs_resolved":
		return packs.RawConfig()

	case "refresh_pixelplay_packs":
		return nil, nil

	case "get_launcher_config":
		return config.Raw()
	case "set_launcher_config":
		cfg, ok := args["config"]
		if !ok {
			return nil, config.Set(json.RawMessage(argsJSON))
		}
		if rt != nil && rt.Discord != nil {
			var parsed map[string]any
			if json.Unmarshal(cfg, &parsed) == nil {
				if v, ok := parsed["enable_discord_presence"].(bool); ok {
					rt.Discord.SetEnabled(v)
				}
				appID, _ := parsed["discord_app_id"].(string)
				img, _ := parsed["discord_large_image"].(string)
				txt, _ := parsed["discord_large_text"].(string)
				rt.Discord.Configure(appID, img, txt)
			}
		}
		return nil, config.Set(cfg)

	case "get_app_version":
		return version.App, nil

	case "get_launcher_directory", "get_data_dir":
		return paths.DataDir(), nil

	case "pick_file", "plugin_dialog_open":
		title, _ := argString(args, "title")
		filter, _ := argString(args, "filter")
		if filter == "" {
			filter = "*.ppack;*.pixelplaypack;*.mrpack;*.zip"
		}
		if title == "" {
			title = "Seleccionar archivo"
		}
		if rt == nil || rt.PickFile == nil {
			return "", fmt.Errorf("sin diálogo de archivos")
		}
		return rt.PickFile(title, filter)

	case "pick_files":
		title, _ := argString(args, "title")
		filter, _ := argString(args, "filter")
		if rt == nil || rt.PickFiles == nil {
			return nil, fmt.Errorf("sin diálogo de archivos")
		}
		return rt.PickFiles(title, filter)

	case "pick_directory":
		title, _ := argString(args, "title")
		if title == "" {
			title = "Seleccionar carpeta"
		}
		if rt == nil || rt.PickDir == nil {
			return "", fmt.Errorf("sin diálogo de carpetas")
		}
		return rt.PickDir(title)

	case "save_file", "plugin_dialog_save":
		title, _ := argString(args, "title")
		suggested, _ := argString(args, "defaultPath", "default_path", "suggested")
		filter, _ := argString(args, "filter")
		if filter == "" {
			filter = "*.ppack"
		}
		if rt == nil || rt.SaveFile == nil {
			return "", fmt.Errorf("sin diálogo de guardar")
		}
		return rt.SaveFile(title, suggested, filter)

	case "open_profile_folder":
		id, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		if err := protect.AssertProfileFolderOpenable(id); err != nil {
			if rt != nil && rt.OnAuthorBlocked != nil {
				rt.OnAuthorBlocked("abrir carpeta del perfil")
			}
			return nil, err
		}
		dir, err := profiles.OpenFolder(id)
		if err != nil {
			return nil, err
		}
		return dir, osopen.Path(dir)

	case "open_file_directory":
		fp, err := argString(args, "filePath", "file_path")
		if err != nil {
			return nil, err
		}
		if err := protect.AssertPathOpenable(fp); err != nil {
			if rt != nil && rt.OnAuthorBlocked != nil {
				rt.OnAuthorBlocked("abrir carpeta de mods")
			}
			return nil, err
		}
		return nil, osopen.Path(fp)

	case "set_discord_state":
		if rt == nil || rt.Discord == nil {
			return nil, nil
		}
		stateType, _ := argString(args, "stateType", "state_type")
		profileName, _ := argString(args, "profileName", "profile_name")
		if discordrpc.IsBrowseKind(stateType) && (rt.Discord.SessionLocked() || process.Any() || launch.IsLaunchingAny()) {
			return nil, nil
		}
		st := discordrpc.State{Kind: stateType}
		if acc, err := accounts.Active(); err == nil && acc != nil {
			st.Player = acc.Username
		}
		switch strings.ToLower(stateType) {
		case "ingame", "in_game", "playing", "playing_game", "launching", "starting", "installing", "downloading":
			p, extra := discordrpc.ParsePipe(profileName)
			st.Profile = p
			st.Pack = p
			st.Version = extra
		default:
			st.Profile = profileName
		}
		_ = rt.Discord.SetState(st)
		return nil, nil

	case "check_for_updates", "check_updates":
		emitUp := func(ev updater.StatusEvent) {
			emit("updater_status", ev)
		}
		man, newer, err := updater.Check(version.App, emitUp)
		if err != nil {
			return nil, err
		}
		if newer && man != nil {
			emit("updater_available", map[string]any{
				"from":    version.App,
				"to":      man.Version,
				"url":     man.URL,
				"notes":   man.Notes,
				"message": "Nueva versión " + man.Version + " disponible",
			})
		}
		return map[string]any{
			"available": newer,
			"manifest":  man,
			"current":   version.App,
			"endpoint":  updater.ManifestURL(version.App),
		}, nil

	case "download_update":
		emitUp := func(ev updater.StatusEvent) {
			emit("updater_status", ev)
		}
		man, newer, err := updater.Check(version.App, emitUp)
		if err != nil {
			return nil, err
		}
		if !newer || man == nil {
			return map[string]any{"downloaded": false, "reason": "up_to_date"}, nil
		}
		dest := filepath.Join(paths.DataDir(), "updates")
		path, err := updater.Download(man, dest, emitUp)
		if err != nil {
			return nil, err
		}
		if err := updater.ApplyAndRelaunch(path); err != nil {
			return map[string]any{"downloaded": true, "path": path, "version": man.Version, "restarting": false, "apply_error": err.Error()}, nil
		}
		emit("updater_status", updater.StatusEvent{
			Status:      "restarting",
			Message:     "Instalando " + man.Version + " y reiniciando…",
			ToVersion:   man.Version,
			FromVersion: version.App,
		})
		go func() {
			time.Sleep(800 * time.Millisecond)
			os.Exit(0)
		}()
		return map[string]any{"downloaded": true, "path": path, "version": man.Version, "restarting": true}, nil

	case "install_vcredist", "repair_vcredist":
		path, err := vcredist.LaunchSetup(nil)
		if err != nil {
			return nil, err
		}
		emit("vcredist_required", map[string]any{
			"url":     vcredist.RedistURL,
			"message": vcredist.PopupMessage,
			"title":   "Instalá Visual C++",
			"path":    path,
		})
		return map[string]any{
			"ok":      true,
			"path":    path,
			"url":     vcredist.RedistURL,
			"message": vcredist.PopupMessage,
			"present": vcredist.Present(),
		}, nil
	case "dismiss_vcredist":
		vcredist.MarkAcknowledged()
		return map[string]any{"ok": true, "present": vcredist.Present()}, nil
	case "vcredist_status":
		return map[string]any{
			"present":      vcredist.Present(),
			"acknowledged": vcredist.Acknowledged(),
			"url":          vcredist.RedistURL,
		}, nil

	case "backend_info":
		return map[string]string{
			"engine":        "wails+go",
			"data_dir":      paths.DataDir(),
			"version":       version.App,
			"updater_base":  updater.DefaultBaseURL,
			"updater_check": updater.ManifestURL(version.App),
		}, nil

	case "get_all_profiles_and_last_played":
		if list, err := profiles.List(); err == nil && len(list) == 0 {
			_, _ = featured.EnsureDefaultProfile()
		}
		raw, err := profiles.RawList()
		if err != nil {
			return nil, err
		}
		var all []json.RawMessage
		if err := json.Unmarshal(raw, &all); err != nil {
			return nil, err
		}
		return map[string]any{
			"all_profiles":           all,
			"last_played_profile_id": nil,
		}, nil

	// --- Auth ---
	case "get_accounts":
		list, err := accounts.List()
		if err != nil {
			return nil, err
		}
		return accounts.ForUIList(list), nil
	case "get_active_account":
		acc, err := accounts.Active()
		if err != nil {
			return nil, err
		}
		if acc == nil {
			return nil, nil
		}
		if !acc.Offline() || acc.PixelPasswordAccount() {
			if updated, err := auth.EnsurePixelplay(*acc); err == nil {
				acc = &updated
			}
		}
		return acc.ForUI(), nil
	case "set_active_account":
		id, err := argString(args, "accountId", "account_id")
		if err != nil {
			return nil, err
		}
		if err := accounts.SetActive(id); err != nil {
			return nil, err
		}
		acc, err := accounts.Active()
		if err != nil || acc == nil {
			return nil, err
		}
		if !acc.Offline() || acc.PixelPasswordAccount() {
			if updated, err := auth.EnsurePixelplay(*acc); err == nil {
				acc = &updated
			}
		}
		return acc.ForUI(), nil
	case "remove_account":
		id, err := argString(args, "accountId", "account_id")
		if err != nil {
			return nil, err
		}
		return nil, accounts.Remove(id)
	case "login_pixel_account":
		user, err := argString(args, "username", "name")
		if err != nil {
			return nil, err
		}
		pass, err := argString(args, "password", "clave")
		if err != nil {
			return nil, err
		}
		creds, err := auth.LoginPixel(user, pass)
		if err != nil {
			return nil, err
		}
		return creds.ForUI(), nil
	case "create_offline_account":
		user, err := argString(args, "username", "name")
		if err != nil {
			return nil, err
		}
		creds, err := accounts.CreateOffline(user, "")
		if err != nil {
			return nil, err
		}
		return creds.ForUI(), nil
	case "lookup_pixel_account":
		user, err := argString(args, "username", "name")
		if err != nil {
			return nil, err
		}
		look, err := auth.LookupPixel(user)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"exists":   look.Exists,
			"username": look.Username,
			"kind":     look.Kind,
			"rank":     look.Rank,
		}, nil
	case "reserve_pixel_account":
		user, err := argString(args, "username", "name")
		if err != nil {
			return nil, err
		}
		pass, err := argString(args, "password", "clave")
		if err != nil {
			return nil, err
		}
		creds, err := auth.ReservePixel(user, pass)
		if err != nil {
			return nil, err
		}
		return creds.ForUI(), nil
	case "list_survival_board":
		sort, _ := argString(args, "sort")
		return auth.ListSurvivalBoard(sort)
	case "get_survival_player":
		user, err := argString(args, "username", "name", "player")
		if err != nil {
			return nil, err
		}
		return auth.GetSurvivalPlayer(user)
	case "list_friends":
		return auth.ListFriends()
	case "add_friend":
		user, err := argString(args, "username", "name")
		if err != nil {
			return nil, err
		}
		return auth.AddFriend(user)
	case "remove_friend":
		user, err := argString(args, "username", "name")
		if err != nil {
			return nil, err
		}
		if err := auth.RemoveFriend(user); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "accept_friend":
		user, err := argString(args, "username", "name")
		if err != nil {
			return nil, err
		}
		return auth.AcceptFriend(user)
	case "decline_friend":
		user, err := argString(args, "username", "name")
		if err != nil {
			return nil, err
		}
		if err := auth.DeclineFriend(user); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "cancel_friend":
		user, err := argString(args, "username", "name")
		if err != nil {
			return nil, err
		}
		if err := auth.CancelFriend(user); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "social_status", "social_send", "social_history", "social_unread", "social_read",
		"social_typing", "social_call", "social_accept", "social_hangup", "social_mute", "social_deafen",
		"social_webrtc", "social_pcm_fallback", "social_mic_level", "social_native_start", "social_native_command", "social_native_stop", "social_native_devices":
		return socialCommand(cmd, args)
	case "desktop_notify":
		title, _ := argString(args, "title")
		body, _ := argString(args, "body", "message")
		if title == "" {
			title = "Pixel Play"
		}
		notify.Desktop(title, body)
		return map[string]any{"ok": true}, nil
	case "refresh_pixel_profile":
		return auth.RefreshPixelProfile()
	case "set_presence":
		status, _ := argString(args, "status")
		if status == "" {
			status = "online"
		}
		return auth.SetActivePresence(status)
	case "create_offline_account_with_id":
		user, err := argString(args, "username", "name")
		if err != nil {
			return nil, err
		}
		id, _ := argString(args, "id")
		creds, err := accounts.CreateOffline(user, id)
		if err != nil {
			return nil, err
		}
		return creds.ForUI(), nil
	case "begin_login":
		open := func(u string) {
			if rt != nil && rt.OpenURL != nil {
				rt.OpenURL(u)
			}
		}
		dev, err := auth.StartLogin(open, func(msg string) {
			emit("state_event", map[string]any{
				"event_type": "account_login",
				"message":    msg,
			})
		})
		if err != nil {
			return nil, err
		}
		emit("msa_device_code", devicePayload(dev))
		creds, err := auth.WaitLogin()
		if err != nil {
			return nil, err
		}
		return persistMinecraftLogin(creds)

	case "start_msa_login":
		open := func(u string) {
			if rt != nil && rt.OpenURL != nil {
				rt.OpenURL(u)
			}
		}
		dev, err := auth.StartLogin(open, func(msg string) {
			emit("state_event", map[string]any{
				"event_type": "account_login",
				"message":    msg,
			})
		})
		if err != nil {
			return nil, err
		}
		payload := devicePayload(dev)
		emit("msa_device_code", payload)
		return payload, nil

	case "wait_msa_login":
		creds, err := auth.WaitLogin()
		if err != nil {
			return nil, err
		}
		return persistMinecraftLogin(creds)

	// --- Play / launch ---
	case "launch_profile":
		id, err := argString(args, "id", "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		opt := launch.Options{}
		opt.QuickPlaySingleplayer, _ = argString(args, "quickPlaySingleplayer", "quick_play_singleplayer")
		opt.QuickPlayMultiplayer, _ = argString(args, "quickPlayMultiplayer", "quick_play_multiplayer")
		var discord *discordrpc.Manager
		if rt != nil {
			discord = rt.Discord
		}
		return nil, launch.Start(id, emit, discord, opt)
	case "abort_profile_launch":
		id, _ := argString(args, "profileId", "profile_id", "id")
		launch.Abort(id)
		return nil, nil
	case "is_profile_launching":
		id, _ := argString(args, "profileId", "profile_id", "id")
		return launch.IsLaunching(id), nil
	case "get_processes", "get_running_processes":
		return process.List(), nil
	case "kill_minecraft":
		id, _ := argString(args, "profileId", "profile_id", "id")
		return nil, process.Kill(id)
	case "stop_process":
		id, _ := argString(args, "processId", "process_id", "id")
		for _, p := range process.List() {
			if p.ID == id {
				return nil, process.Kill(p.ProfileID)
			}
		}
		return nil, process.Kill(id)
	case "open_log_window":
		return nil, nil
	case "get_full_log":
		id, _ := argString(args, "processId", "process_id", "profileId", "profile_id")
		for _, p := range process.List() {
			if p.ID == id || p.ProfileID == id {
				sum, err := profiles.Get(p.ProfileID)
				if err != nil {
					break
				}
				b, err := logs.ReadFile(filepath.Join(paths.ProfileDir(sum.Path), "logs", "latest.log"))
				if err != nil {
					return "", nil
				}
				return b, nil
			}
		}
		return "", nil

	// --- Local content / downloads ---
	case "get_local_content":
		params := map[string]json.RawMessage{}
		if raw, ok := args["params"]; ok {
			_ = json.Unmarshal(raw, &params)
		} else {
			params = args
		}
		pid, err := argString(params, "profile_id", "profileId")
		if err != nil {
			return nil, err
		}
		ct, _ := argString(params, "content_type", "contentType")
		calc := argBool(params, true, "calculate_hashes", "calculateHashes")
		return content.ListLocal(pid, ct, calc)

	case "add_modrinth_content_to_profile", "add_modrinth_mod_to_profile":
		pid, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		fileName, _ := argString(args, "fileName", "file_name")
		dlURL, err := argString(args, "downloadUrl", "download_url")
		if err != nil {
			return nil, err
		}
		ct, _ := argString(args, "contentType", "content_type")
		if ct == "" {
			ct = "Mod"
		}
		dir, err := content.ProfileContentDir(pid, ct)
		if err != nil {
			return nil, err
		}
		if fileName == "" {
			fileName = filepath.Base(dlURL)
		}
		dest := filepath.Join(dir, filepath.Base(fileName))
		emit("state_event", map[string]any{
			"target_id":  pid,
			"event_type": "downloading",
			"message":    "Descargando " + filepath.Base(fileName),
		})
		if err := download.File(dlURL, dest, nil); err != nil {
			return nil, err
		}
		return dest, nil

	// --- Modrinth ---
	case "search_modrinth_projects":
		query, _ := argString(args, "query")
		pt, _ := argString(args, "projectType", "project_type")
		gv, _ := argString(args, "gameVersion", "game_version")
		loader, _ := argString(args, "loader")
		sort, _ := argString(args, "sort")
		limit := argInt(args, 20, "limit")
		offset := argInt(args, 0, "offset")
		var cats []string
		if raw, ok := args["categoriesFilter"]; ok {
			_ = json.Unmarshal(raw, &cats)
		} else if raw, ok := args["categories_filter"]; ok {
			_ = json.Unmarshal(raw, &cats)
		}
		cs, _ := argString(args, "clientSideFilter", "client_side_filter")
		ss, _ := argString(args, "serverSideFilter", "server_side_filter")
		return modrinth.SearchProjects(modrinth.SearchParams{
			Query: query, ProjectType: pt, GameVersion: gv, Loader: loader,
			Limit: limit, Offset: offset, Sort: sort, Categories: cats,
			ClientSideFilter: cs, ServerSideFilter: ss,
		})
	case "search_modrinth_mods":
		query, _ := argString(args, "query")
		gv, _ := argString(args, "gameVersion", "game_version")
		loader, _ := argString(args, "loader")
		limit := argInt(args, 20, "limit")
		return modrinth.SearchMods(query, gv, loader, limit)
	case "get_modrinth_mod_versions":
		id, err := argString(args, "projectIdOrSlug", "project_id_or_slug")
		if err != nil {
			return nil, err
		}
		var loaders, versions []string
		if raw, ok := args["loaders"]; ok {
			_ = json.Unmarshal(raw, &loaders)
		}
		if raw, ok := args["gameVersions"]; ok {
			_ = json.Unmarshal(raw, &versions)
		} else if raw, ok := args["game_versions"]; ok {
			_ = json.Unmarshal(raw, &versions)
		}
		return modrinth.ProjectVersions(id, loaders, versions)
	case "get_modrinth_project_details":
		var ids []string
		if raw, ok := args["ids"]; ok {
			_ = json.Unmarshal(raw, &ids)
		}
		return modrinth.Projects(ids)
	case "get_modrinth_versions_by_hashes":
		var hashes []string
		if raw, ok := args["hashes"]; ok {
			_ = json.Unmarshal(raw, &hashes)
		}
		return modrinth.VersionsByHashes(hashes)
	case "check_modrinth_updates":
		body, ok := args["request"]
		if !ok {
			body = json.RawMessage(argsJSON)
		}
		return modrinth.CheckUpdates(body)
	case "get_modrinth_categories_command":
		return modrinth.Categories()
	case "get_modrinth_loaders_command":
		return modrinth.Loaders()
	case "get_modrinth_game_versions_command":
		return modrinth.GameVersions()

	case "search_mods_unified_command":
		return unified.Search(nestedRaw(args, argsJSON, "params"))
	case "get_mod_versions_unified_command":
		return unified.Versions(nestedRaw(args, argsJSON, "params"))
	case "check_mod_updates_unified_command":
		return unified.CheckUpdates(nestedRaw(args, argsJSON, "request"))
	case "get_modpack_versions_unified_command":
		return unified.ModpackVersions(nestedRaw(args, argsJSON, "modpackSource", "modpack_source"))
	case "get_curseforge_file_changelog_command":
		modID := argID(args, "modId", "mod_id")
		fileID := argID(args, "fileId", "file_id")
		if modID == "" || fileID == "" {
			return nil, fmt.Errorf("faltan modId/fileId")
		}
		return curseforge.Changelog(modID, fileID)
	case "get_curseforge_mods_by_ids":
		var ids []uint64
		if raw, ok := args["modIds"]; ok {
			_ = json.Unmarshal(raw, &ids)
		} else if raw, ok := args["mod_ids"]; ok {
			_ = json.Unmarshal(raw, &ids)
		}
		mods, err := curseforge.ModsByIDs(ids)
		if err != nil {
			return nil, err
		}
		return map[string]any{"data": mods}, nil

	// --- Capes ---
	case "browse_capes":
		payload := args
		if raw, ok := args["payload"]; ok {
			_ = json.Unmarshal(raw, &payload)
		}
		bp := capes.BrowseParams{}
		if v := argIntPtr(payload, "page"); v != nil {
			bp.Page = v
		}
		if v := argIntPtr(payload, "page_size", "pageSize"); v != nil {
			bp.PageSize = v
		}
		bp.SortBy, _ = argString(payload, "sort_by", "sortBy")
		bp.FilterCreator, _ = argString(payload, "filter_creator", "filterCreator")
		bp.TimeFrame, _ = argString(payload, "time_frame", "timeFrame")
		bp.PixelplayToken, _ = argString(payload, "pixelplay_token", "pixelplayToken")
		bp.RequestUUID, _ = argString(payload, "request_uuid", "requestUuid")
		if raw, ok := payload["filter_has_elytra"]; ok {
			var b bool
			if json.Unmarshal(raw, &b) == nil {
				bp.FilterHasElytra = &b
			}
		} else if raw, ok := payload["filterHasElytra"]; ok {
			var b bool
			if json.Unmarshal(raw, &b) == nil {
				bp.FilterHasElytra = &b
			}
		}
		return capes.Browse(bp)
	case "get_player_capes":
		payload := args
		if raw, ok := args["payload"]; ok {
			_ = json.Unmarshal(raw, &payload)
		}
		pid, _ := argString(payload, "playerIdentifier", "player_identifier", "playerUuid", "player_uuid")
		tok, _ := argString(payload, "pixelplayToken", "pixelplay_token")
		return capes.PlayerCapes(pid, tok)
	case "equip_cape":
		hash, err := argString(args, "capeHash", "cape_hash")
		if err != nil {
			return nil, err
		}
		tok, _ := argString(args, "pixelplayToken", "pixelplay_token")
		pu, _ := argString(args, "playerUuid", "player_uuid")
		return nil, capes.Equip(hash, tok, pu)
	case "unequip_cape":
		tok, _ := argString(args, "pixelplayToken", "pixelplay_token")
		pu, _ := argString(args, "playerUuid", "player_uuid")
		return nil, capes.Unequip(tok, pu)

	case "get_standard_profiles":
		return map[string]any{"profiles": []any{}}, nil
	case "refresh_standard_versions":
		return []any{}, nil
	case "set_blocked_mods_config", "log_message_command":
		return nil, nil
	case "get_news_and_changelogs_command":
		return []any{}, nil
	case "get_system_ram_mb":
		return systemRAMMB(), nil
	case "get_minecraft_versions":
		return mcmeta.FetchManifest()
	case "get_fabric_loader_versions":
		mc, err := argString(args, "minecraftVersion", "minecraft_version")
		if err != nil {
			return nil, err
		}
		return mcmeta.FabricLoaderVersions(mc)
	case "get_quilt_loader_versions":
		mc, err := argString(args, "minecraftVersion", "minecraft_version")
		if err != nil {
			return nil, err
		}
		return mcmeta.QuiltLoaderVersions(mc)
	case "get_forge_versions":
		mc, err := argString(args, "minecraftVersion", "minecraft_version")
		if err != nil {
			return nil, err
		}
		return mcmeta.ForgeVersionsFor(mc)
	case "get_neoforge_versions":
		mc, err := argString(args, "minecraftVersion", "minecraft_version")
		if err != nil {
			return nil, err
		}
		return mcmeta.NeoForgeVersionsFor(mc)
	case "resolve_loader_version":
		pid, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		mc, _ := argString(args, "minecraftVersion", "minecraft_version")
		p, err := profiles.Get(pid)
		if err != nil {
			return nil, err
		}
		if mc == "" {
			mc = p.GameVersion
		}
		requested := ""
		if p.LoaderVersion != nil {
			requested = *p.LoaderVersion
		}
		res := mcmeta.ResolveLoaderVersion(p.Loader, requested, mc)
		if requested != "" {
			res.Reason = "user_overwrite"
			v := requested
			res.Version = &v
		}
		return res, nil
	case "validate_java_path_command":
		p, _ := argString(args, "path", "javaPath", "java_path")
		return java.Validate(p), nil
	case "detect_java_installations_command":
		return java.DetectInstallations()
	case "get_java_info_command":
		jp, err := argString(args, "path", "javaPath", "java_path")
		if err != nil {
			return nil, err
		}
		return java.Inspect(jp)
	case "find_best_java_for_minecraft_command":
		mc, err := argString(args, "minecraftVersion", "minecraft_version")
		if err != nil {
			return nil, err
		}
		return java.FindBestForMinecraft(mc)
	case "invalidate_java_cache_command":
		java.InvalidateCache()
		return nil, nil
	case "get_icons_for_archives", "get_icons_for_pixelplay_mods":
		return map[string]any{}, nil
	case "check_for_group_migration_command":
		return map[string]any{
			"direction":   "None",
			"needed":      false,
			"source_path": nil,
			"target_path": nil,
		}, nil
	case "get_profile_latest_log_content":
		id, err := argString(args, "profileId", "profile_id", "id")
		if err != nil {
			return nil, err
		}
		p, err := profiles.Get(id)
		if err != nil {
			return nil, err
		}
		if content, err := logs.LatestForProfile(id); err == nil {
			return content, nil
		}
		logPath := filepath.Join(paths.ProfileDir(p.Path), "logs", "latest.log")
		return logs.ReadFile(logPath)
	case "get_profile_directory_structure":
		id, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		if err := protect.AssertProfileFolderOpenable(id); err != nil {
			if rt != nil && rt.OnAuthorBlocked != nil {
				rt.OnAuthorBlocked("ver carpetas del cliente")
			}
			return nil, err
		}
		p, err := profiles.Get(id)
		if err != nil {
			return nil, err
		}
		root := paths.ProfileDir(p.Path)
		return dirNode(filepath.Base(root), root, 0), nil
	case "is_content_installed", "batch_check_content_installed":
		return map[string]any{"installed": false}, nil
	case "set_file_enabled":
		fp, err := argString(args, "filePath", "file_path")
		if err != nil {
			return nil, err
		}
		if err := protect.AssertPathOpenable(fp); err != nil {
			if rt != nil && rt.OnAuthorBlocked != nil {
				rt.OnAuthorBlocked("modificar archivos del pack")
			}
			return nil, err
		}
		return nil, content.SetEnabled(fp, argBool(args, true, "enabled"))
	case "delete_file":
		fp, err := argString(args, "filePath", "file_path")
		if err != nil {
			return nil, err
		}
		if err := protect.AssertPathOpenable(fp); err != nil {
			if rt != nil && rt.OnAuthorBlocked != nil {
				rt.OnAuthorBlocked("borrar archivos del pack")
			}
			return nil, err
		}
		return nil, content.Delete(fp)
	case "toggle_mod_updates":
		payload := payloadMap(args)
		pid, err := argString(payload, "profile_id", "profileId")
		if err != nil {
			return nil, err
		}
		mid, err := argString(payload, "mod_id", "modId")
		if err != nil {
			return nil, err
		}
		return nil, profiles.SetModUpdates(pid, mid, argBool(payload, true, "updates_enabled", "updatesEnabled", "enabled"))
	case "bulk_toggle_mod_updates":
		payload := payloadMap(args)
		pid, err := argString(payload, "profile_id", "profileId")
		if err != nil {
			return nil, err
		}
		var entries []struct {
			ModID   string `json:"mod_id"`
			Enabled bool   `json:"updates_enabled"`
		}
		if raw, ok := payload["mod_updates"]; ok {
			_ = json.Unmarshal(raw, &entries)
		} else if raw, ok := payload["modUpdates"]; ok {
			_ = json.Unmarshal(raw, &entries)
		}
		return nil, profiles.BulkSetModUpdates(pid, entries)
	case "uninstall_content_from_profile":
		payload := payloadMap(args)
		pid, err := argString(payload, "profile_id", "profileId")
		if err != nil {
			return nil, err
		}
		fp, _ := argString(payload, "file_path", "filePath")
		sha, _ := argString(payload, "sha1_hash", "sha1Hash")
		ct, _ := argString(payload, "content_type", "contentType")
		return nil, content.Uninstall(pid, fp, sha, ct)
	case "toggle_content_from_profile":
		payload := payloadMap(args)
		pid, err := argString(payload, "profile_id", "profileId")
		if err != nil {
			return nil, err
		}
		fp, _ := argString(payload, "file_path", "filePath")
		sha, _ := argString(payload, "sha1_hash", "sha1Hash")
		ct, _ := argString(payload, "content_type", "contentType")
		return nil, content.Toggle(pid, fp, sha, ct, argBool(payload, true, "enabled"))
	case "install_content_to_profile":
		payload := payloadMap(args)
		pid, err := argString(payload, "profile_id", "profileId")
		if err != nil {
			return nil, err
		}
		ct, _ := argString(payload, "content_type", "contentType")
		fname, _ := argString(payload, "file_name", "fileName")
		dl, err := argString(payload, "download_url", "downloadUrl")
		if err != nil {
			return nil, err
		}
		return nil, content.InstallRemote(pid, ct, fname, dl)
	case "set_profile_mod_enabled":
		pid, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		mid, err := argString(args, "modId", "mod_id")
		if err != nil {
			return nil, err
		}
		return nil, profiles.SetModEnabled(pid, mid, argBool(args, true, "enabled"))
	case "delete_mod_from_profile":
		fp, _ := argString(args, "filePath", "file_path", "filename")
		if fp != "" {
			return nil, content.Delete(fp)
		}
		return nil, nil
	case "set_custom_mod_enabled":
		pid, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		fn, err := argString(args, "filename", "fileName")
		if err != nil {
			return nil, err
		}
		dir, err := content.ProfileContentDir(pid, "Mod")
		if err != nil {
			return nil, err
		}
		p := filepath.Join(filepath.Dir(dir), "custom_mods", filepath.Base(fn))
		return nil, content.SetEnabled(p, argBool(args, true, "enabled"))
	case "delete_custom_mod":
		pid, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		fn, err := argString(args, "filename", "fileName")
		if err != nil {
			return nil, err
		}
		dir, err := content.ProfileContentDir(pid, "Mod")
		if err != nil {
			return nil, err
		}
		base := filepath.Join(filepath.Dir(dir), "custom_mods", filepath.Base(fn))
		_ = os.Remove(base)
		_ = os.Remove(base + ".disabled")
		return nil, nil
	case "set_pixelplay_mod_status":
		pid, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		packID, _ := argString(args, "packId", "pack_id")
		modID, _ := argString(args, "modId", "mod_id")
		gv, _ := argString(args, "gameVersion", "game_version")
		loader, _ := argString(args, "loader")
		return nil, profiles.SetPixelplayModStatus(pid, packID, modID, gv, loader, argBool(args, true, "disabled"))
	case "get_custom_mods":
		pid, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		return content.ListCustom(pid)
	case "get_local_resourcepacks", "get_local_shaderpacks", "get_local_datapacks":
		pid, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		ct := "ResourcePack"
		if cmd == "get_local_shaderpacks" {
			ct = "ShaderPack"
		} else if cmd == "get_local_datapacks" {
			ct = "DataPack"
		}
		return content.ListLocal(pid, ct, false)
	case "import_local_mods":
		pid, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		files := []string{}
		if raw, ok := args["paths"]; ok {
			_ = json.Unmarshal(raw, &files)
		}
		if len(files) == 0 && rt != nil && rt.PickFiles != nil {
			files, err = rt.PickFiles("Importar mods", "*.jar")
			if err != nil {
				return nil, err
			}
		}
		dir, err := content.ProfileContentDir(pid, "Mod")
		if err != nil {
			return nil, err
		}
		return nil, content.CopyInto(filepath.Join(filepath.Dir(dir), "custom_mods"), files)
	case "import_profile", "import_profile_from_file":
		fp, _ := argString(args, "filePathStr", "path", "filePath", "file_path")
		if fp == "" {
			if rt == nil || rt.PickFile == nil {
				return nil, fmt.Errorf("falta path del archivo")
			}
			var err error
			fp, err = rt.PickFile("Importar perfil / .ppack", "*.ppack;*.pixelplaypack;*.zip;*.mrpack")
			if err != nil || fp == "" {
				return nil, err
			}
		}
		id, err := importPackAsProfile(fp)
		if err != nil {
			return nil, err
		}
		if cmd == "import_profile" {
			return id, nil
		}
		return map[string]any{"id": id}, nil
	case "install_local_content_to_profile":
		payload := args
		if raw, ok := args["payload"]; ok {
			_ = json.Unmarshal(raw, &payload)
		}
		pid, err := argString(payload, "profile_id", "profileId")
		if err != nil {
			return nil, err
		}
		ct, _ := argString(payload, "content_type", "contentType")
		var files []string
		if raw, ok := payload["file_paths"]; ok {
			_ = json.Unmarshal(raw, &files)
		} else if raw, ok := payload["filePaths"]; ok {
			_ = json.Unmarshal(raw, &files)
		}
		dir, err := content.ProfileContentDir(pid, ct)
		if err != nil {
			return nil, err
		}
		return nil, content.CopyInto(dir, files)
	case "update_modrinth_mod_version", "update_resourcepack_from_modrinth", "update_shaderpack_from_modrinth", "update_datapack_from_modrinth":
		pid, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		ct := "Mod"
		switch cmd {
		case "update_resourcepack_from_modrinth":
			ct = "ResourcePack"
		case "update_shaderpack_from_modrinth":
			ct = "ShaderPack"
		case "update_datapack_from_modrinth":
			ct = "DataPack"
		}
		var details json.RawMessage
		if raw, ok := args["newVersionDetails"]; ok {
			details = raw
		} else if raw, ok := args["new_version_details"]; ok {
			details = raw
		}
		url, name := primaryModrinthFile(details)
		if url == "" {
			return nil, fmt.Errorf("la versión no tiene archivo descargable")
		}
		oldName := ""
		if raw, ok := args["resourcepack"]; ok {
			oldName = jsonFilename(raw)
		} else if raw, ok := args["shaderpack"]; ok {
			oldName = jsonFilename(raw)
		} else if raw, ok := args["datapack"]; ok {
			oldName = jsonFilename(raw)
		}
		return nil, content.UpdateModrinthFile(pid, ct, oldName, url, name)
	case "get_worlds_for_profile":
		id, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		return worlds.ListWorlds(id)
	case "get_servers_for_profile":
		id, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		return worlds.ListServers(id)
	case "ping_minecraft_server":
		addr, err := argString(args, "address")
		if err != nil {
			return nil, err
		}
		return worlds.Ping(addr), nil
	case "copy_world":
		params := args
		if raw, ok := args["params"]; ok {
			_ = json.Unmarshal(raw, &params)
		}
		srcP, err := argString(params, "source_profile_id", "sourceProfileId")
		if err != nil {
			return nil, err
		}
		srcW, err := argString(params, "source_world_folder", "sourceWorldFolder")
		if err != nil {
			return nil, err
		}
		dstP, err := argString(params, "target_profile_id", "targetProfileId")
		if err != nil {
			return nil, err
		}
		name, _ := argString(params, "target_world_name", "targetWorldName")
		return worlds.CopyWorld(srcP, srcW, dstP, name)
	case "delete_world":
		pid, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		folder, err := argString(args, "worldFolder", "world_folder")
		if err != nil {
			return nil, err
		}
		return nil, worlds.DeleteWorld(pid, folder)
	case "check_world_lock_status":
		pid, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		folder, err := argString(args, "worldFolder", "world_folder")
		if err != nil {
			return nil, err
		}
		return worlds.IsLocked(pid, folder)
	case "get_user_skin_data":
		tok, _ := argString(args, "accessToken", "access_token")
		uid, _ := argString(args, "uuid", "id")
		name, _ := argString(args, "username", "name")
		return skins.UserProfile(tok, uid, name)
	case "upload_skin":
		tok, _ := argString(args, "accessToken", "access_token")
		variant, _ := argString(args, "skinVariant", "skin_variant", "variant")
		fp, _ := argString(args, "path", "filePath")
		if fp == "" && rt != nil && rt.PickFile != nil {
			var err error
			fp, err = rt.PickFile("Skin PNG", "*.png")
			if err != nil || fp == "" {
				return nil, err
			}
		}
		b, err := os.ReadFile(fp)
		if err != nil {
			return nil, err
		}
		return nil, skins.ApplyPNG(tok, variant, b)
	case "reset_skin":
		tok, _ := argString(args, "accessToken", "access_token")
		return nil, skins.Reset(tok)
	case "apply_skin_from_base64":
		tok, _ := argString(args, "accessToken", "access_token")
		b64, err := argString(args, "base64Data", "base64_data")
		if err != nil {
			return nil, err
		}
		variant, _ := argString(args, "skinVariant", "skin_variant", "variant")
		return nil, skins.ApplyBase64(tok, variant, b64)
	case "get_all_skins":
		return skins.List()
	case "update_skin_properties":
		id, err := argString(args, "id")
		if err != nil {
			return nil, err
		}
		name, _ := argString(args, "name")
		variant, _ := argString(args, "variant")
		return skins.UpdateProperties(id, name, variant)
	case "remove_skin":
		id, err := argString(args, "id", "skinId")
		if err != nil {
			return nil, err
		}
		return skins.Remove(id)
	case "add_skin_locally":
		payload := args
		if raw, ok := args["payload"]; ok {
			_ = json.Unmarshal(raw, &payload)
		}
		name, _ := argString(payload, "target_skin_name", "targetSkinName", "name")
		variant, _ := argString(payload, "target_skin_variant", "targetSkinVariant", "variant")
		desc, _ := argString(payload, "description")
		src := map[string]any{}
		if raw, ok := payload["source"]; ok {
			_ = json.Unmarshal(raw, &src)
		}
		b64, err := skins.PNGFromSource(src)
		if err != nil {
			return nil, err
		}
		return skins.AddLocal(name, variant, desc, b64)
	case "get_base64_from_skin_source_command":
		src := map[string]any{}
		if raw, ok := args["source"]; ok {
			_ = json.Unmarshal(raw, &src)
		}
		return skins.PNGFromSource(src)
	case "get_starlight_skin_render":
		payload := args
		if raw, ok := args["payload"]; ok {
			_ = json.Unmarshal(raw, &payload)
		}
		player, _ := argString(payload, "player_name", "playerName")
		rtpe, _ := argString(payload, "render_type", "renderType")
		view, _ := argString(payload, "render_view", "renderView")
		b64, _ := argString(payload, "base64_skin_data", "base64SkinData")
		return skins.StarlightRender(player, rtpe, view, b64)
	case "get_owned_vanilla_capes":
		return vanillacapes.Owned()
	case "get_currently_equipped_vanilla_cape":
		return vanillacapes.Equipped()
	case "get_vanilla_cape_info":
		return vanillacapes.Catalog(), nil
	case "equip_vanilla_cape":
		payload := payloadMap(args)
		id, _ := argString(payload, "capeId", "cape_id")
		if id == "" {
			id, _ = argString(args, "capeId", "cape_id")
		}
		return nil, vanillacapes.Equip(id)
	case "refresh_vanilla_cape_data":
		return vanillacapes.Owned()
	case "delete_cape":
		hash, err := argString(args, "capeHash", "cape_hash")
		if err != nil {
			return nil, err
		}
		tok, _ := argString(args, "pixelplayToken", "pixelplay_token")
		pu, _ := argString(args, "playerUuid", "player_uuid")
		return nil, capes.DeleteCape(hash, tok, pu)
	case "add_favorite_cape":
		hash, err := argString(args, "capeHash", "cape_hash")
		if err != nil {
			return nil, err
		}
		tok, _ := argString(args, "pixelplayToken", "pixelplay_token")
		return capes.Favorite(hash, tok, true)
	case "remove_favorite_cape":
		hash, err := argString(args, "capeHash", "cape_hash")
		if err != nil {
			return nil, err
		}
		tok, _ := argString(args, "pixelplayToken", "pixelplay_token")
		return capes.Favorite(hash, tok, false)
	case "get_capes_by_hashes":
		var hashes []string
		if raw, ok := args["hashes"]; ok {
			_ = json.Unmarshal(raw, &hashes)
		}
		tok, _ := argString(args, "pixelplayToken", "pixelplay_token")
		return capes.ByHashes(hashes, tok)
	case "upload_cape":
		tok, _ := argString(args, "pixelplayToken", "pixelplay_token")
		pu, _ := argString(args, "playerUuid", "player_uuid")
		fp, _ := argString(args, "imagePath", "image_path", "path")
		if fp == "" && rt != nil && rt.PickFile != nil {
			var err error
			fp, err = rt.PickFile("Capa PNG", "*.png")
			if err != nil || fp == "" {
				return nil, err
			}
		}
		b, err := os.ReadFile(fp)
		if err != nil {
			return nil, err
		}
		return capes.UploadPNG(b, tok, pu)
	case "download_template_and_open_explorer":
		dir := filepath.Join(paths.DataDir(), "cape-templates")
		if err := capetemplates.WriteTo(dir); err != nil {
			return nil, err
		}
		return dir, osopen.Path(dir)
	case "get_featured_server":
		return featured.InfoDefaults(), nil
	case "ensure_featured_server_profile":
		return featured.Ensure()
	case "get_profile_by_name_or_uuid":
		q, err := argString(args, "nameOrUuidQuery", "name_or_uuid_query", "query")
		if err != nil {
			return nil, err
		}
		return skins.MojangProfile(q)
	case "discord_auth_link", "discord_auth_status", "discord_auth_unlink":
		return map[string]any{"linked": false}, nil
	case "get_mobile_app_token", "reset_mobile_app_token":
		return "", nil
	case "resolve_image_path":
		src, _ := argString(args, "imageSource", "image_source")
		return src, nil
	case "upload_profile_images":
		return "", nil
	case "download_and_install_modrinth_modpack":
		dl, err := argString(args, "downloadUrl", "download_url")
		if err != nil || strings.TrimSpace(dl) == "" {
			return "", fmt.Errorf("falta downloadUrl del .mrpack")
		}
		fname, _ := argString(args, "fileName", "file_name")
		return mrpack.DownloadAndInstall(dl, fname)
	case "switch_content_version", "switch_modpack_version_command":
		return map[string]any{"versions": []any{}}, nil

	case "import_ppack", "import_pixelplay_pack":
		fp, err := argString(args, "path", "filePath", "file_path")
		if err != nil || fp == "" {
			if rt == nil || rt.PickFile == nil {
				return nil, fmt.Errorf("falta path del .ppack")
			}
			fp, err = rt.PickFile("Importar .ppack", "*.ppack;*.pixelplaypack;*.zip")
			if err != nil || fp == "" {
				return nil, err
			}
		}
		m, err := packs.ImportPPackFile(fp)
		if err != nil {
			if profiles.ZipHasProfileJSON(fp) {
				return nil, fmt.Errorf("este archivo es un perfil exportado (profile.json), no un pack de Studio (manifest.json). Importalo desde Perfiles → Importar")
			}
			return nil, err
		}
		return m, nil

	case "get_ppack_policy":
		id, err := argString(args, "packId", "pack_id", "id")
		if err != nil {
			return nil, err
		}
		return packs.PolicyFor(id)

	case "modpack_updater_status":
		id, err := argString(args, "packId", "pack_id", "id")
		if err != nil {
			return nil, err
		}
		up, cur, err := packs.UpdaterFor(id)
		if err != nil {
			return map[string]any{"has_updater": false, "current": ""}, nil
		}
		if up == nil || strings.TrimSpace(up.URL) == "" {
			return map[string]any{"has_updater": false, "current": cur}, nil
		}
		man, newer, err := modpackupdate.Check(cur, up.URL)
		out := map[string]any{
			"has_updater": true,
			"current":     cur,
			"remote":      "",
			"updated":     nil,
		}
		if err != nil {
			out["error"] = err.Error()
			return out, nil
		}
		if man != nil {
			out["remote"] = man.Version
		}
		out["updated"] = !newer
		return out, nil

	case "check_modpack_updates":
		id, err := argString(args, "packId", "pack_id", "id")
		if err != nil {
			return nil, err
		}
		up, cur, err := packs.UpdaterFor(id)
		if err != nil {
			return nil, err
		}
		if up == nil || strings.TrimSpace(up.URL) == "" {
			return map[string]any{"available": false, "current": cur, "endpoint": ""}, nil
		}
		man, newer, err := modpackupdate.Check(cur, up.URL)
		if err != nil {
			return nil, err
		}
		if newer && man != nil {
			emit("modpack_update_available", map[string]any{
				"pack_id": id,
				"from":    cur,
				"to":      man.Version,
				"url":     man.URL,
				"notes":   man.Notes,
			})
		}
		return map[string]any{
			"available": newer,
			"current":   cur,
			"manifest":  man,
			"endpoint":  up.URL,
		}, nil

	case "download_modpack_update":
		id, err := argString(args, "packId", "pack_id", "id")
		if err != nil {
			return nil, err
		}
		up, cur, err := packs.UpdaterFor(id)
		if err != nil {
			return nil, err
		}
		man, newer, err := modpackupdate.Check(cur, up.URL)
		if err != nil {
			return nil, err
		}
		if !newer || man == nil {
			return map[string]any{"downloaded": false, "reason": "up_to_date"}, nil
		}
		path, err := modpackupdate.Download(id, man)
		if err != nil {
			return nil, err
		}
		// Re-import into local packs config
		if _, err := packs.ImportPPackFile(path); err != nil {
			return nil, err
		}
		_ = packs.ApplyOverridesToPackProfiles(id)
		return map[string]any{"downloaded": true, "path": path, "version": man.Version}, nil

	case "export_profile":
		payload := payloadMap(args)
		if raw, ok := args["params"]; ok {
			_ = json.Unmarshal(raw, &payload)
		}
		pid, err := argString(payload, "profile_id", "profileId")
		if err != nil {
			return nil, err
		}
		fname, _ := argString(payload, "file_name", "fileName")
		var include []string
		if raw, ok := payload["include_files"]; ok {
			_ = json.Unmarshal(raw, &include)
		} else if raw, ok := payload["includeFiles"]; ok {
			_ = json.Unmarshal(raw, &include)
		}
		openFolder := argBool(payload, false, "open_folder", "openFolder")
		return profiles.Export(pid, fname, include, openFolder)

	case "get_profile_log_files":
		id, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		return logs.ListForProfile(id)

	case "get_log_file_content":
		p, err := argString(args, "logFilePath", "log_file_path", "path")
		if err != nil {
			return nil, err
		}
		return logs.ReadFile(p)

	case "upload_log_to_mclogs_command":
		contentStr, err := argString(args, "logContent", "log_content", "content")
		if err != nil {
			return nil, err
		}
		return logs.UploadMCLogs(contentStr)

	case "upload_client_log_command":
		payload := payloadMap(args)
		profileID, _ := argString(payload, "profileId", "profile_id")
		exit := argIntPtr(payload, "exitCode", "exit_code")
		logContent, _ := argString(payload, "logContent", "log_content", "latest_log")
		crashContent, _ := argString(payload, "crashReport", "crash_report", "crash_report_content")
		mcName, _ := argString(payload, "mcName", "mc_name", "accountName", "account_name")
		profileName, _ := argString(payload, "profileName", "profile_name")
		pack, _ := argString(payload, "pack", "pixelplay_pack")
		ver, _ := argString(payload, "minecraftVersion", "minecraft_version")
		loader, _ := argString(payload, "loader", "modloader")
		if acc, err := accounts.Active(); err == nil && acc != nil && mcName == "" {
			mcName = acc.Username
		}
		return logs.UploadForProfile(profileID, logs.UploadMeta{
			MCName:           mcName,
			ProfileName:      profileName,
			Pack:             pack,
			MinecraftVersion: ver,
			Loader:           loader,
			ExitCode:         exit,
		}, logs.Snapshot{LatestLog: logContent, CrashReport: crashContent})

	case "get_image_preview":
		payload := payloadMap(args)
		p, err := argString(payload, "path")
		if err != nil {
			return nil, err
		}
		return content.PreviewImage(p)

	case "get_pixelplay_pack_mods":
		id, err := argString(args, "packId", "pack_id")
		if err != nil {
			return nil, err
		}
		return packs.PackMods(id)

	case "list_pixelplay_mods":
		id, err := argString(args, "profileId", "profile_id")
		if err != nil {
			return nil, err
		}
		return packs.ProfilePackMods(id)

	case "import_curseforge_pack":
		p, err := argString(args, "packPath", "pack_path", "path")
		if err != nil {
			return nil, err
		}
		return curseforge.ImportPack(p)

	case "download_and_install_curseforge_modpack_command":
		pid := uint64(argInt(args, 0, "projectId", "project_id"))
		fid := uint64(argInt(args, 0, "fileId", "file_id"))
		fname, _ := argString(args, "fileName", "file_name")
		dl, _ := argString(args, "downloadUrl", "download_url")
		return curseforge.DownloadAndInstall(pid, fid, fname, dl)

	case "submit_crash_log_command":
		acc, _ := accounts.Active()
		if acc != nil && acc.PixelplayCredentials.Production != nil && acc.PixelplayCredentials.Production.Value != "" {
			raw := nestedRaw(args, "{}", "payload")
			var body any
			_ = json.Unmarshal(raw, &body)
			_ = httpx.PostJSONHeader(
				"https://api.pixelplay.gg/api/v1/core/crashlog",
				map[string]string{"Authorization": "Bearer " + acc.PixelplayCredentials.Production.Value},
				body,
				nil,
			)
		}
		return nil, nil

	default:
		return nil, fmt.Errorf("comando no portado aún a Go/Wails: %s", cmd)
	}
}

func systemRAMMB() int {
	if runtime.GOOS == "linux" {
		b, err := os.ReadFile("/proc/meminfo")
		if err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				if strings.HasPrefix(line, "MemTotal:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						kb, _ := strconv.Atoi(fields[1])
						if kb > 0 {
							return kb / 1024
						}
					}
				}
			}
		}
	}
	return 8192
}

func payloadMap(args map[string]json.RawMessage) map[string]json.RawMessage {
	if raw, ok := args["payload"]; ok && len(raw) > 0 && string(raw) != "null" {
		nested := map[string]json.RawMessage{}
		if json.Unmarshal(raw, &nested) == nil && len(nested) > 0 {
			return nested
		}
	}
	return args
}

func nestedRaw(args map[string]json.RawMessage, argsJSON string, keys ...string) json.RawMessage {
	for _, key := range keys {
		if raw, ok := args[key]; ok && len(raw) > 0 && string(raw) != "null" {
			return raw
		}
	}
	return json.RawMessage(argsJSON)
}

func argID(args map[string]json.RawMessage, keys ...string) string {
	if s, err := argString(args, keys...); err == nil && s != "" {
		return s
	}
	if n := argIntPtr(args, keys...); n != nil {
		return strconv.Itoa(*n)
	}
	return ""
}

func persistMinecraftLogin(creds accounts.Credentials) (map[string]any, error) {
	saved, err := accounts.Upsert(creds)
	if err != nil {
		return nil, err
	}
	if updated, err := auth.EnsurePixelplay(saved); err == nil {
		saved = updated
	}
	return saved.ForUI(), nil
}

func devicePayload(dev *auth.DeviceStart) map[string]any {
	return map[string]any{
		"user_code":                 dev.UserCode,
		"verification_uri":          dev.VerificationURI,
		"verification_uri_complete": dev.VerificationURIComplete,
		"message":                   dev.Message,
		"expires_in":                dev.ExpiresIn,
	}
}

func argString(args map[string]json.RawMessage, keys ...string) (string, error) {
	for _, key := range keys {
		raw, ok := args[key]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", fmt.Errorf("argumento %q: %w", key, err)
		}
		return s, nil
	}
	return "", fmt.Errorf("falta argumento %q", keys[0])
}

func argBool(args map[string]json.RawMessage, def bool, keys ...string) bool {
	for _, key := range keys {
		raw, ok := args[key]
		if !ok {
			continue
		}
		var b bool
		if json.Unmarshal(raw, &b) == nil {
			return b
		}
	}
	return def
}

func argInt(args map[string]json.RawMessage, def int, keys ...string) int {
	if v := argIntPtr(args, keys...); v != nil {
		return *v
	}
	return def
}

func argIntPtr(args map[string]json.RawMessage, keys ...string) *int {
	for _, key := range keys {
		raw, ok := args[key]
		if !ok {
			continue
		}
		var n int
		if json.Unmarshal(raw, &n) == nil {
			return &n
		}
		var f float64
		if json.Unmarshal(raw, &f) == nil {
			n = int(f)
			return &n
		}
		var s string
		if json.Unmarshal(raw, &s) == nil {
			if n, err := strconv.Atoi(s); err == nil {
				return &n
			}
		}
	}
	return nil
}

func argInt64(args map[string]json.RawMessage, keys ...string) (int64, error) {
	for _, key := range keys {
		raw, ok := args[key]
		if !ok {
			continue
		}
		var n int64
		if json.Unmarshal(raw, &n) == nil {
			return n, nil
		}
		var f float64
		if json.Unmarshal(raw, &f) == nil {
			return int64(f), nil
		}
		var s string
		if json.Unmarshal(raw, &s) == nil {
			if parsed, err := strconv.ParseInt(s, 10, 64); err == nil {
				return parsed, nil
			}
		}
	}
	return 0, nil
}

func primaryModrinthFile(raw json.RawMessage) (url, name string) {
	var v struct {
		Files []struct {
			URL      string `json:"url"`
			Filename string `json:"filename"`
			Primary  bool   `json:"primary"`
		} `json:"files"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return "", ""
	}
	for _, f := range v.Files {
		if f.Primary {
			return f.URL, f.Filename
		}
	}
	if len(v.Files) > 0 {
		return v.Files[0].URL, v.Files[0].Filename
	}
	return "", ""
}

func jsonFilename(raw json.RawMessage) string {
	var obj struct {
		Filename string `json:"filename"`
		FileName string `json:"file_name"`
		Name     string `json:"name"`
		PathStr  string `json:"path_str"`
	}
	if json.Unmarshal(raw, &obj) != nil {
		return ""
	}
	if obj.Filename != "" {
		return obj.Filename
	}
	if obj.FileName != "" {
		return obj.FileName
	}
	if obj.PathStr != "" {
		return filepath.Base(obj.PathStr)
	}
	return obj.Name
}

func dirNode(name, path string, depth int) map[string]any {
	n := map[string]any{"name": name, "path": path, "children": []any{}}
	if depth > 3 {
		return n
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return n
	}
	kids := make([]any, 0, len(entries))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(path, e.Name())
		if e.IsDir() {
			kids = append(kids, dirNode(e.Name(), full, depth+1))
			continue
		}
		kids = append(kids, map[string]any{"name": e.Name(), "path": full, "children": []any{}})
	}
	n["children"] = kids
	return n
}

func importPackAsProfile(fp string) (string, error) {
	low := strings.ToLower(fp)
	switch {
	case strings.HasSuffix(low, ".mrpack"):
		// .mrpack = Modrinth estándar; Pixel Studio es solo para .ppack.
		return mrpack.ImportFile(fp)
	case strings.HasSuffix(low, ".ppack"), strings.HasSuffix(low, ".pixelplaypack"):
		// Studio = manifest.json; export de perfil del launcher = profile.json
		// (antes el export usaba extensión .ppack por error).
		if profiles.ZipHasProfileJSON(fp) {
			return profiles.ImportFromZip(fp)
		}
		m, err := packs.ImportPPackFile(fp)
		if err != nil {
			return "", err
		}
		name := m.DisplayName
		if name == "" {
			name = m.ID
		}
		gv := m.GameVersion
		if gv == "" {
			gv = "1.21.1"
		}
		loader := m.Loader
		if loader == "" {
			loader = "neoforge"
		}
		return profiles.Create(map[string]any{
			"name":                       name,
			"game_version":               gv,
			"loader":                     loader,
			"selected_pixelplay_pack_id": m.ID,
		})
	case strings.HasSuffix(low, ".zip"):
		// CurseForge zip or generic — try CurseForge first, then treat as unknown.
		if id, err := curseforge.ImportPack(fp); err == nil {
			return id, nil
		}
		if profiles.ZipHasProfileJSON(fp) {
			return profiles.ImportFromZip(fp)
		}
		return "", fmt.Errorf("zip no reconocido: usá .mrpack (Modrinth), .ppack (Pixel Studio) o .pixelplaypack (perfil)")
	default:
		return "", fmt.Errorf("formato no soportado (usá .mrpack, .ppack, .pixelplaypack o zip de CurseForge)")
	}
}
