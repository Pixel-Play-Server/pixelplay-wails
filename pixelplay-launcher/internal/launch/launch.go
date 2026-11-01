package launch

import (
	"bufio"
	"bytes"
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
	"pixelplay-launcher/internal/featured"
	"pixelplay-launcher/internal/java"
	"pixelplay-launcher/internal/logs"
	"pixelplay-launcher/internal/lunar"
	"pixelplay-launcher/internal/mcmeta"
	"pixelplay-launcher/internal/packs"
	"pixelplay-launcher/internal/paths"
	"pixelplay-launcher/internal/playsession"
	"pixelplay-launcher/internal/process"
	"pixelplay-launcher/internal/profiles"
	"pixelplay-launcher/internal/protect"
	"pixelplay-launcher/internal/redact"
	"pixelplay-launcher/internal/vault"
	"pixelplay-launcher/internal/vcredist"
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

func begin(profileID string) (context.Context, json.RawMessage, *accounts.Credentials, error) {
	if IsLaunching(profileID) {
		return nil, nil, nil, fmt.Errorf("el perfil ya se está lanzando")
	}
	raw, err := profiles.RawGet(profileID)
	if err != nil {
		return nil, nil, nil, err
	}
	var native struct {
		HorizonRuntime bool `json:"horizon_runtime"`
	}
	if err := json.Unmarshal(raw, &native); err != nil {
		return nil, nil, nil, err
	}
	// Horizon uses the active PixelPlay identity, with a local guest when none is selected.
	if native.HorizonRuntime {
		acc, err := accounts.Active()
		if err != nil {
			return nil, nil, nil, err
		}
		if acc == nil {
			acc = &accounts.Credentials{Username: "PixelGuest"}
		}
		ctx, cancel := context.WithCancel(context.Background())
		mu.Lock()
		launching[profileID] = true
		cancels[profileID] = cancel
		mu.Unlock()
		return ctx, raw, acc, nil
	}
	acc, err := accounts.Active()
	if err != nil {
		return nil, nil, nil, err
	}
	if acc == nil {
		return nil, nil, nil, fmt.Errorf("no hay cuenta activa — iniciá sesión u offline")
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
	return ctx, raw, acc, nil
}

func Start(profileID string, emit EmitFn, discord *discordrpc.Manager, opt Options) error {
	ctx, raw, acc, err := begin(profileID)
	if err != nil {
		return err
	}
	go func() {
		defer setLaunching(profileID, false)
		if err := run(ctx, profileID, raw, acc, emit, discord, opt); err != nil {
			if err == vcredist.ErrNeedSetup {
				// Popup already emitted via emitVCRedistRequired.
			} else {
				emitState(emit, profileID, "error", err.Error())
			}
			if discord != nil {
				_ = discord.SetState(discordrpc.State{Kind: "idle"})
			}
		}
	}()
	return nil
}

// RunBlocking launches a profile and waits until Minecraft exits.
func RunBlocking(profileID string, emit EmitFn, discord *discordrpc.Manager, opt Options) error {
	ctx, raw, acc, err := begin(profileID)
	if err != nil {
		return err
	}
	defer setLaunching(profileID, false)
	if err := run(ctx, profileID, raw, acc, emit, discord, opt); err != nil {
		if err != vcredist.ErrNeedSetup {
			emitState(emit, profileID, "error", err.Error())
		}
		return err
	}
	return nil
}

type profileJSON struct {
	HorizonRuntime          bool    `json:"horizon_runtime"`
	LunarRuntime            bool    `json:"lunar_runtime"`
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
	if p.HorizonRuntime {
		return runHorizon(ctx, profileID, p, acc, emit, opt)
	}
	if p.LunarRuntime {
		if lunar.Enabled != "private" || p.SelectedPixelplayPackID != nil && *p.SelectedPixelplayPackID != "" {
			return fmt.Errorf("Lunar requiere la compilación privada y un perfil independiente")
		}
		if _, err := lunar.RuntimeForVersion(p.GameVersion); err != nil {
			return err
		}
		if !acc.Offline() {
			refreshed, err := auth.RefreshMinecraftIfNeeded(*acc)
			if err != nil {
				return fmt.Errorf("sesión Microsoft para Lunar: %w", err)
			}
			acc = &refreshed
		}
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

	packID := ""
	if p.SelectedPixelplayPackID != nil {
		packID = *p.SelectedPixelplayPackID
	}
	if packID != "" {
		up, _, uerr := packs.UpdaterFor(packID)
		if uerr == nil && up != nil && strings.TrimSpace(up.URL) != "" {
			step("checking_pack_update", "Buscando actualización del pack…")
			_, to, applied, uerr := packs.ApplyRemoteUpdate(packID, emit)
			if uerr != nil {
				return fmt.Errorf("actualizar pack: %w", uerr)
			}
			if applied {
				step("checking_pack_update", "Pack actualizado a "+to)
				if fresh, err := profiles.RawGet(profileID); err == nil {
					var np profileJSON
					if json.Unmarshal(fresh, &np) == nil {
						p.GameVersion, p.Loader, p.LoaderVersion = np.GameVersion, np.Loader, np.LoaderVersion
					}
				}
			}
		}
	}
	if packID == featured.PackID && strings.EqualFold(p.Loader, "neoforge") {
		v := featured.NeoForgeVersion
		if p.LoaderVersion == nil || strings.TrimSpace(*p.LoaderVersion) != v {
			p.LoaderVersion = &v
			_ = profiles.Update(profileID, map[string]any{"loader_version": v})
		}
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
	if p.LunarRuntime {
		runtimeVersion, err := lunar.RuntimeForVersion(p.GameVersion)
		if err != nil {
			return err
		}
		javaReq = runtimeVersion.Java
	}
	javaBin := ""
	if p.Settings.UseCustomJava && p.Settings.JavaPath != nil && *p.Settings.JavaPath != "" {
		javaBin = *p.Settings.JavaPath
		if p.LunarRuntime && !java.MatchesMajor(javaBin, javaReq) {
			return fmt.Errorf("Lunar %s requiere Java %d; desactivá la ruta Java personalizada", p.GameVersion, javaReq)
		}
	} else {
		ensureJava := java.Ensure
		if p.LunarRuntime {
			ensureJava = java.EnsureExact
		}
		javaBin, err = ensureJava(javaReq, func(m string) { step("installing_java", m) })
		if err != nil {
			return err
		}
	}
	if err := check(); err != nil {
		return err
	}

	protected := false
	if packID != "" {
		if pol, err := packs.PolicyFor(packID); err == nil && pol != nil {
			protected = pol.IsProtectedByAuthor
			if opt.QuickPlayMultiplayer == "" {
				if addr := strings.TrimSpace(pol.AutoJoinServer); addr != "" {
					opt.QuickPlayMultiplayer = addr
				}
			}
		}
	}

	persistDir := paths.ProfileDir(p.Path)
	storeDir := persistDir
	if protected && packID != "" {
		storeDir = paths.VaultDir(packID)
		_ = os.MkdirAll(storeDir, 0o700)
		_ = vault.MigrateFrom(persistDir, storeDir, packID)
	}
	_ = os.MkdirAll(filepath.Join(storeDir, "mods"), 0o755)
	gameDir := persistDir

	step("downloading_libraries", "Descargando libraries…")
	jars, err := mcmeta.DownloadLibraries(meta, nil, func(m string) { step("downloading_libraries", m) })
	if err != nil {
		return err
	}
	if err := check(); err != nil {
		return err
	}

	step("extracting_natives", "Instalando natives…")
	// Genesis verifies and extracts its own version-specific native bundle.
	// Vanilla's modern LWJGL checks reject the legacy 1.8.9 library layout.
	if !p.LunarRuntime {
		if err := mcmeta.EnsureNatives(meta, nil, func(m string) { step("extracting_natives", m) }); err != nil {
			return err
		}
	}
	// Windows: LWJGL needs VC++ runtime — missing → exit 3221225781 (0xC0000135).
	if err := vcredist.Ensure(mcmeta.NativesDir(meta.ID), func(m string) {
		step("extracting_natives", m)
	}); err != nil {
		if err == vcredist.ErrNeedSetup {
			emitVCRedistRequired(emit, profileID)
			return err
		}
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

	if p.LunarRuntime {
		loader = "lunar"
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
		// NeoForge/Forge use BootstrapLauncher + module path (-p). Merging vanilla
		// JVM args (and a giant -cp) hides the minecraft/neoforge game modules →
		// "Actual version: [MISSING]" from ModSorter.
		if loader == "neoforge" || loader == "forge" {
			meta.Arguments.JVM = loaderArgs.JVM
		} else {
			meta.Arguments.JVM = append(meta.Arguments.JVM, loaderArgs.JVM...)
		}
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
		if err := mcmeta.EnsureNatives(&mcmeta.PistonMeta{ID: meta.ID, Libraries: extraLibs}, nil, func(m string) {
			step("extracting_natives", m)
		}); err != nil {
			return err
		}
		_ = vcredist.StageInto(mcmeta.NativesDir(meta.ID))
	}

	if p.SelectedPixelplayPackID != nil && *p.SelectedPixelplayPackID != "" {
		step("downloading_mods", "Descargando mods del pack…")
		modsDir := filepath.Join(storeDir, "mods")
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

	runtimeDir := ""
	if protected && packID != "" {
		step("launching_minecraft", "Preparando instancia cifrada…")
		_ = vault.SealFrom(storeDir, storeDir, packID)
		rd, err := vault.NewRuntimeDir()
		if err != nil {
			return err
		}
		runtimeDir = rd
		defer func() { _ = vault.ShredDir(runtimeDir) }()
		if err := vault.UnsealTo(storeDir, runtimeDir, packID); err != nil {
			return fmt.Errorf("abrir pack cifrado: %w", err)
		}
		staged, err := vault.StageJava(runtimeDir, javaBin)
		if err != nil {
			return err
		}
		javaBin = staged
		gameDir = runtimeDir
		protect.HardenGameDir(gameDir)
		protect.DisableScreenshotKey(gameDir)
	}

	step("launching_minecraft", "Lanzando Minecraft…")
	clientJar := mcmeta.ClientJar(meta.ID)
	// NeoForge/Forge already ship Minecraft as the named `minecraft` module
	// (client-*-srg.jar). Putting vanilla versions/1.21.1/1.21.1.jar on the
	// classpath makes JPMS treat it as automatic module `_1._21._1` and
	// crash with split-package ResolutionException (fzzy_config, DualCodecs…).
	if loader == "neoforge" || loader == "forge" {
		// Vanilla / versions/<loader>/<loader>.jar must NOT be on -cp: FML loads
		// minecraft from client-*-srg via libraryDirectory. Putting those jars on
		// the legacy classpath creates a duplicate "client"/"minecraft" module
		// (ResolutionException) or automatic-module `_1._21._1`.
		jars = dropJar(jars, mcmeta.ClientJar(p.GameVersion))
		if loaderID != "" {
			jars = dropJar(jars, mcmeta.ClientJar(loaderID))
			clientJar = "" // BootstrapLauncher uses -p + legacyClassPath, not a client jar
		}
	}
	cp := classpath(jars, clientJar)
	natives := mcmeta.NativesDir(meta.ID)
	libDir := mcmeta.LibrariesDir()
	if abs, err := filepath.Abs(libDir); err == nil {
		libDir = abs
	}
	if abs, err := filepath.Abs(natives); err == nil {
		natives = abs
	}
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
		s = strings.ReplaceAll(s, "${library_directory}", libDir)
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
	// NeoForge/Forge still need -cp: BootstrapLauncher.loadLegacyClassPath() reads
	// java.class.path. Without it the JVM exits 1 immediately (NoSuchElementException)
	// and never writes logs/latest.log. Keep NeoForge's own -p args; do not merge
	// vanilla's -cp template (see Arguments.JVM replace above).
	if !hasCP && cp != "" {
		args = append(args, "-cp", cp)
	}
	// CustomSkinLoader Universal 15.x can fail SkinManager patches on NeoForge SRG
	// ("matched protocol 0 but did not modify any bytecode") → exit 255 with no window.
	// Official escape hatch; skins may need ForgeV3 CSL, but the game must boot.
	if loader == "neoforge" || loader == "forge" {
		args = append(args, "-Dcustomskinloader.ignorePatchFailure=true")
	}
	args = append(args, fmt.Sprintf("-Dpixelplay.profile.name=%s", p.Name))
	if protected {
		if gameDir != "" {
			args = append(args, "-Dpixelplay.protect.gamedir="+gameDir)
		}
		if agent := protect.JVMAgentArg(gameDir); agent != "" {
			args = append(args, agent)
		}
	}
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
	args = setGameFlagOnce(args, "--quickPlayMultiplayer", opt.QuickPlayMultiplayer)
	args = setGameFlagOnce(args, "--quickPlaySingleplayer", opt.QuickPlaySingleplayer)

	if p.LunarRuntime {
		args, err = lunar.Prepare(ctx, lunar.Options{Version: p.GameVersion, Guest: acc.Offline(), Expires: acc.Expires, ClientJar: mcmeta.ClientJar(meta.ID), GameDir: gameDir, AssetsDir: mcmeta.AssetsDir(), AssetIndex: meta.AssetIndex.ID, Username: acc.Username, UUID: acc.ID, AccessToken: acc.AccessToken, Memory: mem}, func(m string) { step("installing_lunar", m) })
		if err != nil {
			return err
		}
	}

	bin := java.ForGameLaunch(javaBin)
	cmdArgs := args
	if p.Settings.Hooks.Wrapper != nil && *p.Settings.Hooks.Wrapper != "" {
		cmdArgs = append([]string{bin}, args...)
		bin = *p.Settings.Hooks.Wrapper
	}

	cmd := exec.CommandContext(ctx, bin, cmdArgs...)
	cmd.Dir = gameDir
	hideGameConsole(cmd)
	if protected && gameDir != "" {
		guardBin, _ := protect.StageOpenWrappers(gameDir)
		cmd.Env = protect.WithGuardEnv(os.Environ(), gameDir, guardBin)
	}
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return err
	}

	var consoleBuf bytes.Buffer
	var consoleMu sync.Mutex
	appendConsole := func(line string) {
		consoleMu.Lock()
		defer consoleMu.Unlock()
		if consoleBuf.Len() > 200_000 {
			return
		}
		consoleBuf.WriteString(line)
		consoleBuf.WriteByte('\n')
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
	packName := p.Name
	if p.SelectedPixelplayPackID != nil {
		packName = packs.DisplayNameOf(*p.SelectedPixelplayPackID, p.Name)
	}
	playerName := ""
	if acc != nil {
		playerName = acc.Username
	}
	reportPlay := func(line string) {
		if host, ok := discordrpc.ParseConnecting(line); ok {
			playsession.Report(playsession.Event{Type: "connect", Host: host, Pack: packName, Profile: p.Name, Player: playerName})
			return
		}
		if discordrpc.IsDisconnect(line) {
			playsession.Report(playsession.Event{Type: "disconnect", Pack: packName, Profile: p.Name, Player: playerName})
		}
	}
	go discordrpc.WatchLatestLog(watchCtx, filepath.Join(gameDir, "logs", "latest.log"), func(line string) {
		line = redact.Line(line, gameDir)
		if discord != nil {
			discord.HandleGameLog(line)
		}
		reportPlay(line)
	})
	if protected && cmd.Process != nil {
		protect.GuardSession(watchCtx, cmd.Process.Pid, gameDir)
		go protect.ScrubLogs(watchCtx, gameDir)
	}

	go drain(stdout, emit, profileID, discord, gameDir, reportPlay, appendConsole)
	go drain(stderr, emit, profileID, discord, gameDir, reportPlay, appendConsole)

	err = cmd.Wait()
	snap := logs.CaptureGameDir(gameDir, profileID)
	consoleMu.Lock()
	consoleOut := consoleBuf.String()
	consoleMu.Unlock()
	if strings.TrimSpace(snap.LatestLog) == "" && strings.TrimSpace(consoleOut) != "" {
		snap.LatestLog = consoleOut
	} else if strings.TrimSpace(consoleOut) != "" && !strings.Contains(snap.LatestLog, consoleOut[:min(80, len(consoleOut))]) {
		// Keep JVM/stderr (often the only clue on early exit) at the end of the snapshot.
		snap.LatestLog = strings.TrimSpace(snap.LatestLog) + "\n--- consola JVM ---\n" + consoleOut
	}
	if protected && packID != "" && runtimeDir != "" {
		_ = vault.SealFrom(gameDir, storeDir, packID)
	}
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = 1
		}
	}
	stopped := process.TakeStopped(profileID)
	process.Remove(profileID)
	success := code == 0 || stopped
	exitHint := vcredist.ExplainExit(code)
	if !success && !stopped && vcredist.IsDLLMissingExit(code) {
		errRepair := vcredist.RepairAfterCrash(natives, func(m string) {
			step("extracting_natives", m)
		})
		// Only show the VC++ popup if we actually opened the setup.
		if errRepair == vcredist.ErrNeedSetup {
			emitVCRedistRequired(emit, profileID)
		} else {
			// VC++ already there — force re-extract natives next launch by wiping health marker layout.
			_ = os.RemoveAll(natives)
			_ = mcmeta.EnsureNatives(meta, nil, func(m string) { step("extracting_natives", m) })
			_ = vcredist.StageInto(natives)
		}
	}
	exitPayload := map[string]any{
		"profile_id":           profileID,
		"process_id":           metaProc.ID,
		"exit_code":            code,
		"exit_hint":            exitHint,
		"success":              success,
		"stopped":              stopped,
		"process_metadata":     metaProc,
		"crash_report_content": snap.CrashReport,
		"latest_log_content":   logs.ClipTail(snap.LatestLog, 120_000),
		"console_output":       logs.ClipTail(consoleOut, 80_000),
	}
	if emit != nil {
		msg, _ := json.Marshal(exitPayload)
		emit("state_event", map[string]any{
			"event_id":   uuid.NewString(),
			"event_type": "minecraft_process_exited",
			"target_id":  profileID,
			"message":    string(msg),
			"progress":   nil,
			"error":      nil,
		})
		emit("minecraft_process_exited", exitPayload)
	}
	if !success && !stopped {
		codeCopy := code
		go func() {
			_, _ = logs.UploadClient(snap, logs.UploadMeta{
				MCName:           deref(metaProc.AccountName),
				UUID:             deref(metaProc.AccountUUID),
				ProfileName:      deref(metaProc.ProfileName),
				Pack:             deref(metaProc.PixelplayPack),
				MinecraftVersion: deref(metaProc.MinecraftVersion),
				Loader:           deref(metaProc.Modloader),
				ExitCode:         &codeCopy,
				ProcessID:        metaProc.ID,
			})
		}()
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

func drain(r io.Reader, emit EmitFn, profileID string, discord *discordrpc.Manager, gameDir string, reportPlay func(string), onLine func(string)) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := redact.Line(sc.Text(), gameDir)
		if onLine != nil {
			onLine(line)
		}
		if discord != nil {
			discord.HandleGameLog(line)
		}
		if reportPlay != nil {
			reportPlay(line)
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

// setGameFlagOnce keeps a single --flag value. Minecraft rejects the same
// option twice (quickPlayMultiplayer / quickPlaySingleplayer).
func setGameFlagOnce(args []string, flag, value string) []string {
	if strings.TrimSpace(value) == "" && !hasGameFlag(args, flag) {
		return args
	}
	out := make([]string, 0, len(args)+2)
	skipVal := false
	for _, a := range args {
		if skipVal {
			skipVal = false
			continue
		}
		if a == flag {
			skipVal = true
			continue
		}
		out = append(out, a)
	}
	if strings.TrimSpace(value) != "" {
		out = append(out, flag, strings.TrimSpace(value))
	}
	return out
}

func hasGameFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
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

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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

func emitVCRedistRequired(emit EmitFn, profileID string) {
	if emit == nil {
		return
	}
	payload := map[string]any{
		"profile_id": profileID,
		"url":        vcredist.RedistURL,
		"message":    vcredist.PopupMessage,
		"title":      "Instalá Visual C++",
	}
	emit("vcredist_required", payload)
	emit("state_event", map[string]any{
		"event_id":   uuid.NewString(),
		"event_type": "vcredist_required",
		"target_id":  profileID,
		"message":    vcredist.PopupMessage,
		"progress":   nil,
		"error":      nil,
	})
}
