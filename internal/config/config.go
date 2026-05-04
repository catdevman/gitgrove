package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

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
	CacheDir string            `toml:"cache_dir,omitempty"`
	Groves   map[string]*Grove `toml:"groves"`
}

// EffectiveCacheDir returns the configured cache directory, or the default
// (~/.local/share/gitgrove/repos) when none is set.
func (c *Config) EffectiveCacheDir() (string, error) {
	if c.CacheDir != "" {
		return c.CacheDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "gitgrove", "repos"), nil
}

// IsRemoteSource reports whether source is a remote URL rather than a local path.
func IsRemoteSource(source string) bool {
	return strings.HasPrefix(source, "https://") ||
		strings.HasPrefix(source, "git@") ||
		strings.HasPrefix(source, "ssh://")
}

// RemoteClonePath returns the local path where remoteURL should be cloned,
// mirroring the URL structure: cacheDir/host/owner/repo.
func RemoteClonePath(cacheDir, remoteURL string) (string, error) {
	host, repoPath, err := parseRemoteURL(remoteURL)
	if err != nil {
		return "", err
	}
	parts := append([]string{cacheDir, host}, strings.Split(repoPath, "/")...)
	return filepath.Join(parts...), nil
}

func parseRemoteURL(remoteURL string) (host, repoPath string, err error) {
	if strings.HasPrefix(remoteURL, "git@") {
		// git@github.com:owner/repo.git
		rest := strings.TrimPrefix(remoteURL, "git@")
		idx := strings.Index(rest, ":")
		if idx == -1 {
			return "", "", fmt.Errorf("invalid git@ URL: %s", remoteURL)
		}
		host = rest[:idx]
		repoPath = strings.TrimSuffix(rest[idx+1:], ".git")
		return
	}
	u, err := url.Parse(remoteURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid URL %s: %w", remoteURL, err)
	}
	host = u.Host
	repoPath = strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), ".git")
	return
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
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	cfg.CacheDir = expandHome(cfg.CacheDir, home)
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
