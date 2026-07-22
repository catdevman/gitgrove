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

// Save writes the config to the given path, creating parent directories as
// needed. The write is atomic: the config is encoded to a temporary file in the
// same directory and then renamed over the target, so an interrupted write
// cannot leave a truncated or empty config behind.
func Save(cfg *Config, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("could not create config directory: %w", err)
	}

	// Paths are expanded on load; contract them back so a config written as
	// "~/code/backend" survives a round trip instead of being rewritten
	// absolute.
	out := contractPaths(cfg)

	f, err := os.CreateTemp(dir, ".config-*.toml")
	if err != nil {
		return fmt.Errorf("could not write config: %w", err)
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op once the rename succeeds

	if err := toml.NewEncoder(f).Encode(out); err != nil {
		f.Close()
		return fmt.Errorf("could not encode config: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("could not flush config: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("could not write config: %w", err)
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		return fmt.Errorf("could not set config permissions: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("could not replace config: %w", err)
	}
	return nil
}

// contractPaths returns a deep copy of cfg with $HOME-prefixed paths rewritten
// back to "~/" form. The copy keeps the in-memory config expanded so callers
// that continue using it after a save still see absolute paths.
func contractPaths(cfg *Config) *Config {
	home, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}
	out := &Config{
		CacheDir: contractHome(cfg.CacheDir, home),
		Groves:   make(map[string]*Grove, len(cfg.Groves)),
	}
	for name, g := range cfg.Groves {
		ng := &Grove{
			Path:  contractHome(g.Path, home),
			Repos: make([]Repo, len(g.Repos)),
		}
		copy(ng.Repos, g.Repos)
		for i := range ng.Repos {
			ng.Repos[i].Source = contractHome(ng.Repos[i].Source, home)
		}
		out.Groves[name] = ng
	}
	return out
}

func expandHome(path, home string) string {
	if len(path) >= 2 && path[:2] == "~/" {
		return filepath.Join(home, path[2:])
	}
	return path
}

// contractHome is the inverse of expandHome: it rewrites a path under home to
// use the "~/" prefix. Remote sources and paths outside home are left alone.
func contractHome(path, home string) string {
	if path == "" || home == "" || IsRemoteSource(path) {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~/" + filepath.ToSlash(path[len(home)+1:])
	}
	return path
}
