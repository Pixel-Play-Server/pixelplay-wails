package config

import (
	"encoding/json"
	"fmt"
	"os"

	"pixelplay-launcher/internal/paths"
)

// Raw returns launcher_config.json or a minimal default object.
func Raw() (json.RawMessage, error) {
	data, err := os.ReadFile(paths.ConfigFile())
	if err != nil {
		if os.IsNotExist(err) {
			return json.RawMessage(`{}`), nil
		}
		return nil, fmt.Errorf("leer launcher_config: %w", err)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("launcher_config.json inválido")
	}
	return json.RawMessage(data), nil
}

// Set writes launcher_config.json from a JSON object.
func Set(raw json.RawMessage) error {
	if !json.Valid(raw) {
		return fmt.Errorf("config JSON inválido")
	}
	var pretty any
	if err := json.Unmarshal(raw, &pretty); err != nil {
		return err
	}
	out, err := json.MarshalIndent(pretty, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(paths.ConfigFile(), out, 0o644)
}
