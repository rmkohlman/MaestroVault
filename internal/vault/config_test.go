package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.TouchID != false {
		t.Error("default TouchID should be false")
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	dir := t.TempDir()

	// Override Dir() by setting a temp config path.
	path := filepath.Join(dir, "config.json")

	cfg := Config{TouchID: true}

	data, err := marshalConfig(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	loaded, err := loadConfigFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.TouchID != true {
		t.Error("expected TouchID to be true after load")
	}
}

func TestLoadConfigMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	cfg, err := loadConfigFrom(path)
	if err != nil {
		t.Fatalf("expected no error for missing config, got: %v", err)
	}
	if cfg.TouchID != false {
		t.Error("missing config should default to TouchID=false")
	}
}

func TestLoadConfigInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := os.WriteFile(path, []byte("not json{{{"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := loadConfigFrom(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
