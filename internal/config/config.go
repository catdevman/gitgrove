package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Repo is a single repository entry within a grove.
type Repo struct {
	Name   string `toml:"name"`
	Source string `toml:"source"`
	Branch string `toml:"branch"`
}

// Grove is a named workspace containing linked worktrees from multiple repos.
type Grove struct {
	Path  string `toml:"path"`
	Repos []Repo `toml:"repos"`
}

// Config is the top-level configuration.
type Config struct {
	Groves map[string]*Grove `toml:"groves"`
}

// DefaultPath returns the default config file path (~/.config/gitgrove/config.toml).
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "gitgrove", "config.toml"), nil
}

// Load reads the config file at the given path.
// If the file does not exist, an empty Config is returned.
func Load(path string) (*Config, error) {
	cfg := &Config{Groves: make(map[string]*Grove)}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("could not decode config at %s: %w", path, err)
	}
	if cfg.Groves == nil {
		cfg.Groves = make(map[string]*Grove)
	}
	// Expand ~ in grove paths.
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	for _, g := range cfg.Groves {
		g.Path = expandHome(g.Path, home)
		for i := range g.Repos {
			g.Repos[i].Source = expandHome(g.Repos[i].Source, home)
		}
	}
	return cfg, nil
}

// Save writes the config to the given path, creating parent directories as needed.
func Save(cfg *Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("could not create config directory: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not write config: %w", err)
	}
	defer f.Close()
	enc := toml.NewEncoder(f)
	return enc.Encode(cfg)
}

func expandHome(path, home string) string {
	if len(path) >= 2 && path[:2] == "~/" {
		return filepath.Join(home, path[2:])
	}
	return path
}
