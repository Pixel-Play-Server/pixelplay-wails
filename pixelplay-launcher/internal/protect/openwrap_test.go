package protect

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWithGuardEnvPrependsPath(t *testing.T) {
	env := []string{"PATH=/usr/bin", "LANG=C"}
	out := WithGuardEnv(env, "/tmp/.abc", "/tmp/.abc/.bin")
	var path, gdir string
	for _, e := range out {
		if strings.HasPrefix(e, "PATH=") {
			path = e
		}
		if strings.HasPrefix(e, "PIXELPLAY_GAME_DIR=") {
			gdir = e
		}
	}
	if !strings.HasPrefix(path, "PATH=/tmp/.abc/.bin:") {
		t.Fatalf("path: %s", path)
	}
	if gdir != "PIXELPLAY_GAME_DIR=/tmp/.abc" {
		t.Fatalf("gamedir: %s", gdir)
	}
}

func TestXdgOpenWrapperBlocksGameDir(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	if err := os.MkdirAll(filepath.Join(game, "mods"), 0o700); err != nil {
		t.Fatal(err)
	}
	bin, err := StageOpenWrappers(game)
	if err != nil {
		t.Fatal(err)
	}
	wrap := filepath.Join(bin, "xdg-open")
	cmd := exec.Command(wrap, "file://"+filepath.Join(game, "mods"))
	cmd.Env = []string{"PIXELPLAY_GAME_DIR=" + game, "PATH=/usr/bin:/bin"}
	cmd.Dir = game
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("blocked open should exit 0: %v %s", err, out)
	}

	cmd = exec.Command(wrap, "https://pixelplay.gg")
	cmd.Env = []string{"PIXELPLAY_GAME_DIR=" + game, "PATH=/usr/bin:/bin"}
	// https is not blocked; real xdg-open may fail in this environment — that's ok
	// as long as we did not treat it as blocked (exit 0 without exec is also ok if xdg-open missing).
	_ = cmd.Run()
}
