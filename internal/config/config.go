package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const DefaultConfigFileName = ".worktree-manager.yml"

type Config struct {
	Include []string `yaml:"include"`
	Exclude []string `yaml:"exclude"`
}

func Default() Config {
	return Config{
		Include: []string{".env", ".env.*", "**/.env", "**/.env.*"},
		Exclude: []string{"**/*.example*", "**/node_modules/**", "**/.git/**"},
	}
}

type Loaded struct {
	Config Config
	Source string // "defaults" or absolute file path
}

func Load(repoRoot string) (Loaded, error) {
	path := filepath.Join(repoRoot, DefaultConfigFileName)
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Loaded{Config: Default(), Source: "defaults"}, nil
		}
		return Loaded{}, fmt.Errorf("failed to read config %s: %w", path, err)
	}

	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return Loaded{}, fmt.Errorf("failed to parse %s: %w", path, err)
	}

	// If user provides an empty config file, keep behavior sane.
	if len(c.Include) == 0 {
		c.Include = Default().Include
	}
	if len(c.Exclude) == 0 {
		c.Exclude = Default().Exclude
	}

	return Loaded{Config: c, Source: path}, nil
}

