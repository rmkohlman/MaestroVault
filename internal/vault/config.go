package vault

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const configFileName = "config.json"

// Config holds user-facing vault configuration, persisted as JSON.
type Config struct {
	// TouchID enables biometric authentication when opening the vault.
	TouchID bool `json:"touchid"`
	// VimMode enables vim keybindings (Normal/Visual/Insert) in the TUI.
	VimMode bool `json:"vim_mode"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		TouchID: false,
	}
}

// ConfigPath returns the full path to the config file.
func ConfigPath() string {
	return filepath.Join(Dir(), configFileName)
}

// LoadConfig reads the config file from the vault directory.
// If the file does not exist, it returns DefaultConfig (no error).
func LoadConfig() (Config, error) {
	return loadConfigFrom(ConfigPath())
}

// SaveConfig writes the config to the vault directory.
func SaveConfig(cfg Config) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating vault directory: %w", err)
	}

	data, err := marshalConfig(cfg)
	if err != nil {
		return err
	}

	path := ConfigPath()
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

// loadConfigFrom reads a config file at an arbitrary path.
func loadConfigFrom(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return Config{}, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}

// marshalConfig serializes a Config to JSON bytes.
func marshalConfig(cfg Config) ([]byte, error) {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling config: %w", err)
	}
	return append(data, '\n'), nil
}
