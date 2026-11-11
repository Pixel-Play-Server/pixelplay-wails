package launch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"pixelplay-launcher/internal/accounts"
	"pixelplay-launcher/internal/auth"
	"pixelplay-launcher/internal/horizon"
	"regexp"

	"strings"

	"pixelplay-launcher/internal/paths"
	"pixelplay-launcher/internal/process"
	"pixelplay-launcher/internal/profiles"
)

// Horizon runs directly. PixelPlay owns preparation and the selected identity.
func horizonCommand(ctx context.Context, p profileJSON, opt Options, native horizon.Runtime, acc *accounts.Credentials) (*exec.Cmd, error) {
	if !horizon.SupportedPlatform() {
		return nil, fmt.Errorf("este perfil Horizon requiere Linux x86_64, Windows x86_64/ARM64 o macOS Intel/Apple Silicon")
	}
	if p.LunarRuntime || p.SelectedPixelplayPackID != nil && *p.SelectedPixelplayPackID != "" {
		return nil, fmt.Errorf("Horizon requiere un perfil independiente de Lunar y de packs Java")
	}
	if p.Path == "" || filepath.IsAbs(p.Path) || filepath.Clean(p.Path) == ".." || strings.HasPrefix(filepath.Clean(p.Path), ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("carpeta Horizon inválida")
	}
	binary := native.Binary
	if _, err := os.Stat(binary); err != nil {
		return nil, fmt.Errorf("motor Horizon no instalado: %w", err)
	}
	if acc == nil || !regexp.MustCompile(`^[A-Za-z0-9_]{3,16}$`).MatchString(acc.Username) {
		return nil, fmt.Errorf("elige en PixelPlay un nombre de 3 a 16 letras, números o _")
	}
	gameDir := paths.ProfileDir(p.Path)
	if opt.QuickPlayMultiplayer != "" && opt.QuickPlaySingleplayer != "" {
		return nil, fmt.Errorf("elige un mundo o un servidor para Horizon")
	}
	args := []string{"--resources", native.Resources, "--name", acc.Username}
	if opt.QuickPlayMultiplayer != "" {
		args = append(args, "--server", opt.QuickPlayMultiplayer)
	}
	if opt.QuickPlaySingleplayer != "" {
		args = append(args, "--world", opt.QuickPlaySingleplayer)
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = gameDir
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "HORIZON_") {
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	cmd.Env = append(cmd.Env, "HORIZON_HOME="+gameDir)
	return cmd, nil
}

func runHorizon(ctx context.Context, id string, p profileJSON, acc *accounts.Credentials, emit EmitFn, opt Options) error {
	if p.Path == "" || filepath.IsAbs(p.Path) || filepath.Clean(p.Path) == ".." || strings.HasPrefix(filepath.Clean(p.Path), ".."+string(filepath.Separator)) {
		return fmt.Errorf("carpeta Horizon inválida")
	}
	gameDir := paths.ProfileDir(p.Path)
	if horizonRunning(gameDir) {
		return fmt.Errorf("Horizon ya está abierto con este perfil; cierra esa partida antes de iniciar otra")
	}
	native, err := horizon.Ensure(ctx, gameDir, func(msg string) { emitState(emit, id, "installing_game", msg) })
	if err != nil {
		return err
	}
	if !acc.Offline() {
		refreshed, err := auth.RefreshMinecraftIfNeeded(*acc)
		if err != nil {
			return fmt.Errorf("sesión Microsoft de PixelPlay: %w", err)
		}
		acc = &refreshed
	}
	cmd, err := horizonCommand(ctx, p, opt, native, acc)
	if err != nil {
		return err
	}
	cleanup, err := horizonSession(cmd, acc)
	if err != nil {
		return err
	}
	defer cleanup()
	stop := configureHorizonProcess(cmd)
	if err := os.MkdirAll(filepath.Join(cmd.Dir, "logs"), 0700); err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(cmd.Dir, "logs", "horizon-launcher.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd.Stdout, cmd.Stderr = log, log
	emitState(emit, id, "launching", "Iniciando Horizon con la cuenta seleccionada en PixelPlay")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("abrir Horizon: %w", err)
	}
	process.AddWithStop(cmd.Process, process.Metadata{ProfileID: id, ProfileName: &p.Name, MinecraftVersion: &p.GameVersion, AccountName: &acc.Username, AccountUUID: &acc.ID}, stop)
	profiles.TouchLastPlayed(id)
	emitState(emit, id, "launch_successful", "Horizon abierto")
	err = cmd.Wait()
	stopped := process.TakeStopped(id)
	process.Remove(id)
	code := 0
	if err != nil && !stopped {
		code = 1
	}
	payload := map[string]any{"profile_id": id, "exit_code": code, "success": code == 0, "stopped": stopped}
	if emit != nil {
		msg, _ := json.Marshal(payload)
		emitState(emit, id, "minecraft_process_exited", string(msg))
		emit("minecraft_process_exited", payload)
	}
	if code != 0 {
		return fmt.Errorf("Horizon terminó con error; revisa logs/horizon-launcher.log. La cuenta y los recursos se gestionan desde PixelPlay. Detalle: %w", err)
	}
	return nil
}

// The engine consumes and deletes this private, single-use session file.
// Offline launches never create a session or pass a token.
func horizonSession(cmd *exec.Cmd, acc *accounts.Credentials) (func(), error) {
	noop := func() {}
	if acc.Offline() {
		return noop, nil
	}
	dir := filepath.Join(cmd.Dir, ".horizon-native", "sessions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return noop, err
	}
	f, err := os.CreateTemp(dir, "session-*.json")
	if err != nil {
		return noop, err
	}
	cleanup := func() { _ = os.Remove(f.Name()) }
	err = json.NewEncoder(f).Encode(map[string]string{"name": acc.Username, "uuid": acc.ID, "access_token": acc.AccessToken, "user_type": "msa"})
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		cleanup()
		return noop, err
	}
	cmd.Args = append(cmd.Args, "--session-file", f.Name())
	return cleanup, nil
}
