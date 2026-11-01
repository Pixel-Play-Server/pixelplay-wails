package nativertc

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

//go:embed engine.py
var script []byte

type Engine struct {
	mu    sync.Mutex
	cmd   *exec.Cmd
	input io.WriteCloser
}

func command(extra ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("el motor multimedia nativo es para Linux")
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(cache, "pixelplay", "rtc")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "engine.py")
	if err = os.WriteFile(path, script, 0600); err != nil {
		return nil, err
	}
	cmd := exec.Command("/usr/bin/python3", append([]string{path}, extra...)...)
	cmd.Env = os.Environ()
	home, _ := os.UserHomeDir()
	local := filepath.Join(home, ".local", "share", "pixelplay", "gstreamer", "runtime", "usr", "lib", "x86_64-linux-gnu")
	for name, sub := range map[string]string{"GST_PLUGIN_PATH": "gstreamer-1.0", "GI_TYPELIB_PATH": "girepository-1.0"} {
		p := filepath.Join(local, sub)
		if _, err := os.Stat(p); err == nil {
			if existing := os.Getenv(name); existing != "" {
				p += string(os.PathListSeparator) + existing
			}
			cmd.Env = append(cmd.Env, name+"="+p)
		}
	}
	return cmd, nil
}
func Devices() (any, error) {
	cmd, err := command("--devices")
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("motor multimedia: %s", stderr.String())
	}
	var devices any
	err = json.Unmarshal(data, &devices)
	return devices, err
}
func (e *Engine) Start(settings any, callback func(map[string]any)) error {
	e.Stop()
	cmd, err := command()
	if err != nil {
		return err
	}
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		return err
	}
	e.mu.Lock()
	e.cmd = cmd
	e.input = input
	e.mu.Unlock()
	go func() {
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 4096), 2*1024*1024)
		for scanner.Scan() {
			var event map[string]any
			if json.Unmarshal(scanner.Bytes(), &event) == nil {
				callback(event)
			}
		}
		if err := cmd.Wait(); err != nil {
			e.mu.Lock()
			current := e.cmd == cmd
			e.mu.Unlock()
			if current {
				callback(map[string]any{"event": "error", "error": "El motor de audio/video se cerró: " + err.Error()})
			}
		}
	}()
	return e.Send(settings)
}
func (e *Engine) Send(command any) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.input == nil {
		return fmt.Errorf("el motor de llamada no está iniciado")
	}
	return json.NewEncoder(e.input).Encode(command)
}
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.input != nil {
		_ = e.input.Close()
	}
	if e.cmd != nil && e.cmd.Process != nil {
		_ = e.cmd.Process.Kill()
	}
	e.input = nil
	e.cmd = nil
}
