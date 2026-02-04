package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsWhenMissing(t *testing.T) {
	dir := t.TempDir()
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if loaded.Source != "defaults" {
		t.Fatalf("expected defaults source, got %q", loaded.Source)
	}
	if len(loaded.Config.Include) == 0 || len(loaded.Config.Exclude) == 0 {
		t.Fatalf("expected non-empty defaults, got %#v", loaded.Config)
	}
}

func TestLoadParsesYaml(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DefaultConfigFileName)
	if err := os.WriteFile(path, []byte("include:\n  - apps/*/.env\nexclude:\n  - \"**/*.example*\"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if loaded.Source != path {
		t.Fatalf("expected %q, got %q", path, loaded.Source)
	}
	if len(loaded.Config.Include) != 1 || loaded.Config.Include[0] != "apps/*/.env" {
		t.Fatalf("unexpected include: %#v", loaded.Config.Include)
	}
}

