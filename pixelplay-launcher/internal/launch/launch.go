package launch

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	"pixelplay-launcher/internal/accounts"
	"pixelplay-launcher/internal/auth"
	"pixelplay-launcher/internal/discordrpc"
	"pixelplay-launcher/internal/java"
	"pixelplay-launcher/internal/mcmeta"
	"pixelplay-launcher/internal/packs"
	"pixelplay-launcher/internal/paths"
	"pixelplay-launcher/internal/process"
	"pixelplay-launcher/internal/profiles"
)

type EmitFn func(event string, payload any)

type Options struct {
	QuickPlaySingleplayer string
	QuickPlayMultiplayer  string
}

var (
	mu        sync.Mutex
	launching = map[string]bool{}
	cancels   = map[string]context.CancelFunc{}
)

func IsLaunching(profileID string) bool {
	mu.Lock()
	defer mu.Unlock()
	return launching[profileID]
}

func IsLaunchingAny() bool {
	mu.Lock()
	defer mu.Unlock()
	return len(launching) > 0
}

func Abort(profileID string) {
	mu.Lock()
	c := cancels[profileID]
	mu.Unlock()
	if c != nil {
		c()
	}
	_ = process.Kill(profileID)
}

func setLaunching(profileID string, v bool) {
	mu.Lock()
	defer mu.Unlock()
	if v {
		launching[profileID] = true
	} else {
		delete(launching, profileID)
		delete(cancels, profileID)
	}
}

func Start(profileID string, emit EmitFn, discord *discordrpc.Manager, opt Options) error {
	if IsLaunching(profileID) {
		return fmt.Errorf("el perfil ya se está lanzando")
	}
	raw, err := profiles.RawGet(profileID)
	if err != nil {
		return err
	}
	acc, err := accounts.Active()
	if err != nil {
		return err
	}
	if acc == nil {
		return fmt.Errorf("no hay cuenta activa — iniciá sesión u offline")
	}
	if !acc.Offline() {
		if updated, err := auth.EnsurePixelplay(*acc); err == nil {
			acc = &updated
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	mu.Lock()
	launching[profileID] = true
	cancels[profileID] = cancel
	mu.Unlock()

	go func() {
		defer setLaunching(profileID, false)
		if err := run(ctx, profileID, raw, acc, emit, discord, opt); err != nil {
			emitState(emit, profileID, "error", err.Error())
			if discord != nil {
				_ = discord.SetState(discordrpc.State{Kind: "idle"})
			}
		}
	}()
	return nil
}

type profileJSON struct {
	ID                      string  `json:"id"`
	Name                    string  `json:"name"`
	Path                    string  `json:"path"`
	GameVersion             string  `json:"game_version"`
	Loader                  string  `json:"loader"`
	LoaderVersion           *string `json:"loader_version"`
	SelectedPixelplayPackID *string `json:"selected_pixelplay_pack_id"`
	DisabledPixelplayMods   []struct {
		PackID      string `json:"pack_id"`
		ModID       string `json:"mod_id"`
		GameVersion string `json:"game_version"`
		Loader      string `json:"loader"`
	} `json:"disabled_pixelplay_mods_detailed"`
	Settings struct {
		Memory struct {
			Max int `json:"max"`
		} `json:"memory"`
		ExtraGameArgs []string `json:"extra_game_args"`
		CustomJVMArgs *string  `json:"custom_jvm_args"`
		JavaPath      *string  `json:"java_path"`
		UseCustomJava bool     `json:"use_custom_java_path"`
		Hooks         struct {
			Wrapper *string `json:"wrapper"`
		} `json:"hooks"`
	} `json:"settings"`
}

func run(ctx context.Context, profileID string, raw json.RawMessage, acc *accounts.Credentials, emit EmitFn, discord *discordrpc.Manager, opt Options) error {
	var p profileJSON
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	check := func() error {
		select {
		case <-ctx.Done():
			return fmt.Errorf("lanzamiento cancelado")
		default:
			return nil
		}
	}

	if discord != nil {
		_ = discord.SetState(discordPresence(p, acc, opt, "launching"))
	}

	step := func(typ, msg string) {
		emitState(emit, profileID, typ, msg)
	}

	step("installing_java", "Preparando Java…")
	javaReq := 21
	meta, err := mcmeta.FetchPiston(p.GameVersion)
	if err != nil {
		return err
	}
	if meta.JavaVersion.MajorVersion > 0 {
		javaReq = meta.JavaVersion.MajorVersion
	}
	javaBin := ""
	if p.Settings.UseCustomJava && p.Settings.JavaPath != nil && *p.Settings.JavaPath != "" {
		javaBin = *p.Settings.JavaPath
	} else {
		javaBin, err = java.Ensure(javaReq, func(m string) { step("installing_java", m) })
		if err != nil {
			return err
		}
	}
	if err := check(); err != nil {
		return err
	}

	gameDir := paths.ProfileDir(p.Path)
	_ = os.MkdirAll(filepath.Join(gameDir, "mods"), 0o755)

	step("downloading_libraries", "Descargando libraries…")
	jars, err := mcmeta.DownloadLibraries(meta, nil, func(m string) { step("downloading_libraries", m) })
	if err != nil {
		return err
	}
	if err := check(); err != nil {
		return err
	}

	step("extracting_natives", "Extrayendo natives…")
	if err := mcmeta.ExtractNatives(meta, nil); err != nil {
		return err
	}

	step("downloading_assets", "Descargando assets…")
	if err := mcmeta.DownloadAssets(meta, func(m string) { step("downloading_assets", m) }); err != nil {
		return err
	}

	step("downloading_client", "Descargando cliente Minecraft…")
	if err := mcmeta.DownloadClient(meta); err != nil {
		return err
	}
	if err := check(); err != nil {
		return err
	}

	mainClass := meta.MainClass
	extraLibs := []mcmeta.Library{}
	var loaderArgs *mcmeta.Arguments
	loaderID := ""
	loader := strings.ToLower(p.Loader)
	loaderVer := ""
	if p.LoaderVersion != nil {
		loaderVer = *p.LoaderVersion
	}

	switch loader {
	case "fabric":
		step("installing_fabric", "Instalando Fabric…")
		fp, libs, err := mcmeta.InstallFabric(p.GameVersion, loaderVer, func(m string) { step("installing_fabric", m) })
		if err != nil {
			return err
		}
		if fp.MainClass != "" {
			mainClass = fp.MainClass
		}
		extraLibs = libs
		loaderArgs = fp.Arguments
		loaderID = fp.ID
	case "quilt":
		step("installing_quilt", "Instalando Quilt…")
		qp, libs, err := mcmeta.InstallQuilt(p.GameVersion, loaderVer, func(m string) { step("installing_quilt", m) })
		if err != nil {
			return err
		}
		if qp.MainClass != "" {
			mainClass = qp.MainClass
		}
		extraLibs = libs
		loaderArgs = qp.Arguments
		loaderID = qp.ID
	case "neoforge":
		step("installing_neoforge", "Instalando NeoForge…")
		nf, err := mcmeta.InstallNeoForge(p.GameVersion, loaderVer, javaBin, func(m string) { step("installing_neoforge", m) })
		if err != nil {
			return err
		}
		if nf.MainClass != "" {
			mainClass = nf.MainClass
		}
		extraLibs = nf.Libraries
		loaderVer = nf.Version
		loaderArgs = nf.Arguments
		loaderID = nf.ID
	case "forge":
		step("installing_forge", "Instalando Forge…")
		fg, err := mcmeta.InstallForge(p.GameVersion, loaderVer, javaBin, func(m string) { step("installing_forge", m) })
		if err != nil {
			return err
		}
		if fg.MainClass != "" {
			mainClass = fg.MainClass
		}
		extraLibs = fg.Libraries
		loaderVer = fg.Version
		loaderArgs = fg.Arguments
		loaderID = fg.ID
	}

	if loaderArgs != nil && meta.Arguments != nil {
		meta.Arguments.JVM = append(meta.Arguments.JVM, loaderArgs.JVM...)
		meta.Arguments.Game = append(meta.Arguments.Game, loaderArgs.Game...)
	} else if loaderArgs != nil {
		meta.Arguments = loaderArgs
	}

	if len(extraLibs) > 0 {
		more, err := mcmeta.DownloadLibraries(&mcmeta.PistonMeta{Libraries: extraLibs}, nil, nil)
		if err != nil {
			return err
		}
		jars = append(more, jars...)
		_ = mcmeta.ExtractNatives(&mcmeta.PistonMeta{ID: meta.ID, Libraries: extraLibs}, nil)
	}

	if p.SelectedPixelplayPackID != nil && *p.SelectedPixelplayPackID != "" {
		step("downloading_mods", "Descargando mods del pack…")
		modsDir := filepath.Join(gameDir, "mods")
		skip := map[string]bool{}
		for _, d := range p.DisabledPixelplayMods {
			if d.PackID == *p.SelectedPixelplayPackID &&
				(d.GameVersion == "" || d.GameVersion == p.GameVersion) &&
				(d.Loader == "" || strings.EqualFold(d.Loader, loader)) {
				skip[d.ModID] = true
			}
		}
		if err := packs.SyncPackMods(*p.SelectedPixelplayPackID, p.GameVersion, loader, modsDir, skip, func(m string) {
			step("downloading_mods", m)
		}); err != nil {
			return err
		}
		step("syncing_mods", "Mods sincronizados")
	}

	step("launching_minecraft", "Lanzando Minecraft…")
	clientJar := mcmeta.ClientJar(meta.ID)
	// NeoForge/Forge already ship Minecraft as the named `minecraft` module
	// (client-*-srg.jar). Putting vanilla versions/1.21.1/1.21.1.jar on the
	// classpath makes JPMS treat it as automatic module `_1._21._1` and
	// crash with split-package ResolutionException (fzzy_config, DualCodecs…).
	if loader == "neoforge" || loader == "forge" {
		vanillaClient := mcmeta.ClientJar(p.GameVersion)
		jars = dropJar(jars, vanillaClient)
		if loaderID != "" {
			clientJar = mcmeta.ClientJar(loaderID)
		}
	}
	cp := classpath(jars, clientJar)
	natives := mcmeta.NativesDir(meta.ID)
	mem := p.Settings.Memory.Max
	if mem <= 0 {
		mem = 4096
	}

	verName := p.GameVersion
	if loaderID != "" {
		verName = loaderID
	}
	repl := func(s string) string {
		s = strings.ReplaceAll(s, "${natives_directory}", natives)
		s = strings.ReplaceAll(s, "${launcher_name}", "pixelplayclient-launcher")
		s = strings.ReplaceAll(s, "${launcher_version}", "3.0.0")
		s = strings.ReplaceAll(s, "${classpath}", cp)
		s = strings.ReplaceAll(s, "${auth_player_name}", acc.Username)
		s = strings.ReplaceAll(s, "${version_name}", verName)
		s = strings.ReplaceAll(s, "${game_directory}", gameDir)
		s = strings.ReplaceAll(s, "${assets_root}", mcmeta.AssetsDir())
		s = strings.ReplaceAll(s, "${game_assets}", mcmeta.AssetsDir())
		s = strings.ReplaceAll(s, "${assets_index_name}", meta.AssetIndex.ID)
		s = strings.ReplaceAll(s, "${auth_uuid}", acc.ID)
		tok := acc.AccessToken
		if tok == "" {
			tok = "0"
		}
		s = strings.ReplaceAll(s, "${auth_access_token}", tok)
		s = strings.ReplaceAll(s, "${clientid}", "c4502edb-87c6-40cb-b595-64a280cf8906")
		s = strings.ReplaceAll(s, "${auth_xuid}", "0")
		userType := "msa"
		if acc.AccessToken == "" {
			userType = "legacy"
		}
		s = strings.ReplaceAll(s, "${user_type}", userType)
		s = strings.ReplaceAll(s, "${version_type}", meta.Type)
		s = strings.ReplaceAll(s, "${user_properties}", "{}")
		s = strings.ReplaceAll(s, "${library_directory}", mcmeta.LibrariesDir())
		s = strings.ReplaceAll(s, "${classpath_separator}", string(os.PathListSeparator))
		if (loader == "neoforge" || loader == "forge") && strings.Contains(s, "-DignoreList=") {
			vanillaName := p.GameVersion + ".jar"
			if vanillaName != ".jar" && !strings.Contains(s, vanillaName) {
				s = strings.Replace(s, "-DignoreList=", "-DignoreList="+vanillaName+",", 1)
			}
		}
		return s
	}

	args := []string{}
	hasCP := false
	hasNatives := false
	if meta.Arguments != nil {
		args = append(args, flattenArgs(meta.Arguments.JVM, repl)...)
	}
	for _, a := range args {
		if a == "-cp" {
			hasCP = true
		}
		if strings.HasPrefix(a, "-Djava.library.path=") {
			hasNatives = true
		}
	}
	args = append(args, fmt.Sprintf("-Xmx%dM", mem))
	if p.Settings.CustomJVMArgs != nil {
		args = append(args, strings.Fields(*p.Settings.CustomJVMArgs)...)
	}
	if !hasNatives {
		args = append(args, "-Djava.library.path="+natives)
	}
	if !hasCP {
		args = append(args, "-cp", cp)
	}
	args = append(args, fmt.Sprintf("-Dpixelplay.profile.name=%s", p.Name))
	args = append(args, mainClass)
	if meta.Arguments != nil {
		args = append(args, flattenArgs(meta.Arguments.Game, repl)...)
	} else if meta.MinecraftArguments != "" {
		for _, tok := range strings.Fields(meta.MinecraftArguments) {
			args = append(args, repl(tok))
		}
	} else {
		args = append(args,
			"--username", acc.Username,
			"--version", p.GameVersion,
			"--gameDir", gameDir,
			"--assetsDir", mcmeta.AssetsDir(),
			"--assetIndex", meta.AssetIndex.ID,
			"--uuid", acc.ID,
			"--accessToken", repl("${auth_access_token}"),
			"--userType", repl("${user_type}"),
			"--versionType", meta.Type,
		)
	}
	args = append(args, p.Settings.ExtraGameArgs...)
	if opt.QuickPlayMultiplayer != "" {
		args = append(args, "--quickPlayMultiplayer", opt.QuickPlayMultiplayer)
	}
	if opt.QuickPlaySingleplayer != "" {
		args = append(args, "--quickPlaySingleplayer", opt.QuickPlaySingleplayer)
	}

	bin := javaBin
	cmdArgs := args
	if p.Settings.Hooks.Wrapper != nil && *p.Settings.Hooks.Wrapper != "" {
		cmdArgs = append([]string{javaBin}, args...)
		bin = *p.Settings.Hooks.Wrapper
	}

	cmd := exec.CommandContext(ctx, bin, cmdArgs...)
	cmd.Dir = gameDir
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return err
	}

	metaProc := process.Metadata{
		ID:               uuid.NewString(),
		ProfileID:        profileID,
		MinecraftVersion: &p.GameVersion,
		Modloader:        &p.Loader,
		ProfileName:      &p.Name,
		AccountName:      &acc.Username,
		AccountUUID:      &acc.ID,
	}
	if loaderVer != "" {
		metaProc.ModloaderVersion = &loaderVer
	}
	if p.SelectedPixelplayPackID != nil {
		metaProc.PixelplayPack = p.SelectedPixelplayPackID
	}
	process.Add(cmd.Process, metaProc)
	profiles.TouchLastPlayed(profileID)

	step("launch_successful", "Minecraft iniciado")
	if discord != nil {
		_ = discord.SetState(discordPresence(p, acc, opt, "ingame"))
	}

	watchCtx, stopWatch := context.WithCancel(context.Background())
	defer stopWatch()
	go discordrpc.WatchLatestLog(watchCtx, filepath.Join(gameDir, "logs", "latest.log"), func(line string) {
		if discord != nil {
			discord.HandleGameLog(line)
		}
	})

	go drain(stdout, emit, profileID, discord)
	go drain(stderr, emit, profileID, discord)

	err = cmd.Wait()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = 1
		}
	}
	process.Remove(profileID)
	success := code == 0
	if emit != nil {
		emit("state_event", map[string]any{
			"event_id":   uuid.NewString(),
			"event_type": "minecraft_process_exited",
			"target_id":  profileID,
			"message":    fmt.Sprintf("Minecraft salió (%d)", code),
			"progress":   nil,
			"error":      nil,
		})
		emit("minecraft_process_exited", map[string]any{
			"profile_id": profileID,
			"process_id": metaProc.ID,
			"exit_code":  code,
			"success":    success,
		})
	}
	if discord != nil {
		st := discordPresence(p, acc, opt, "idle")
		st.Server = ""
		_ = discord.SetState(st)
	}
	return nil
}

func discordPresence(p profileJSON, acc *accounts.Credentials, opt Options, kind string) discordrpc.State {
	st := discordrpc.State{
		Kind:    kind,
		Profile: p.Name,
		Version: p.GameVersion,
		Pack:    p.Name,
	}
	if p.SelectedPixelplayPackID != nil {
		st.Pack = packs.DisplayNameOf(*p.SelectedPixelplayPackID, p.Name)
	}
	if acc != nil {
		st.Player = acc.Username
	}
	if opt.QuickPlayMultiplayer != "" && (kind == "ingame" || kind == "launching") {
		st.Server = opt.QuickPlayMultiplayer
	}
	return st
}

func drain(r io.Reader, emit EmitFn, profileID string, discord *discordrpc.Manager) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if discord != nil {
			discord.HandleGameLog(line)
		}
		if emit != nil {
			emit("minecraft_output", map[string]any{
				"event_type": "minecraft_output",
				"target_id":  profileID,
				"message":    line,
			})
		}
	}
}

func dropJar(jars []string, drop string) []string {
	if drop == "" {
		return jars
	}
	drop = filepath.Clean(drop)
	base := filepath.Base(drop)
	out := jars[:0]
	for _, j := range jars {
		cj := filepath.Clean(j)
		if cj == drop || filepath.Base(cj) == base && strings.Contains(cj, string(filepath.Separator)+"versions"+string(filepath.Separator)) {
			continue
		}
		out = append(out, j)
	}
	return out
}

func classpath(jars []string, client string) string {
	seen := map[string]bool{}
	var parts []string
	for _, j := range jars {
		if j == "" || seen[j] {
			continue
		}
		seen[j] = true
		parts = append(parts, j)
	}
	if client != "" && !seen[client] {
		parts = append(parts, client)
	}
	return strings.Join(parts, string(os.PathListSeparator))
}

func flattenArgs(raw []json.RawMessage, repl func(string) string) []string {
	var out []string
	for _, item := range raw {
		var s string
		if json.Unmarshal(item, &s) == nil {
			out = append(out, repl(s))
			continue
		}
		var obj struct {
			Rules []mcmeta.Rule   `json:"rules"`
			Value json.RawMessage `json:"value"`
		}
		if json.Unmarshal(item, &obj) != nil {
			continue
		}
		if !mcmeta.RulesAllow(obj.Rules) {
			continue
		}
		var one string
		if json.Unmarshal(obj.Value, &one) == nil {
			out = append(out, repl(one))
			continue
		}
		var many []string
		if json.Unmarshal(obj.Value, &many) == nil {
			for _, v := range many {
				out = append(out, repl(v))
			}
		}
	}
	return out
}

func emitState(emit EmitFn, profileID, eventType, message string) {
	if emit == nil {
		return
	}
	emit("state_event", map[string]any{
		"event_id":   uuid.NewString(),
		"event_type": eventType,
		"target_id":  profileID,
		"message":    message,
		"progress":   nil,
		"error": func() any {
			if eventType == "error" {
				return message
			}
			return nil
		}(),
	})
}
