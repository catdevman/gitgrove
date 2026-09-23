package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/catdevman/gitgrove/internal/dirs"
)

func TestIsRemoteSource(t *testing.T) {
	for _, tc := range []struct {
		source string
		want   bool
	}{
		{"https://github.com/org/repo", true},
		{"git@github.com:org/repo.git", true},
		{"ssh://git@github.com/org/repo", true},
		{"~/code/backend", false},
		{"/home/me/code/backend", false},
		{"", false},
		{"http://github.com/org/repo", false}, // only https is recognised
	} {
		if got := IsRemoteSource(tc.source); got != tc.want {
			t.Errorf("IsRemoteSource(%q) = %v, want %v", tc.source, got, tc.want)
		}
	}
}

func TestRemoteClonePath(t *testing.T) {
	cache := filepath.Join("/cache", "repos")
	for _, tc := range []struct {
		url  string
		want string
	}{
		{"https://github.com/org/repo", filepath.Join(cache, "github.com", "org", "repo")},
		{"https://github.com/org/repo.git", filepath.Join(cache, "github.com", "org", "repo")},
		{"git@github.com:org/repo.git", filepath.Join(cache, "github.com", "org", "repo")},
		{"git@gitlab.com:group/sub/repo", filepath.Join(cache, "gitlab.com", "group", "sub", "repo")},
		{"ssh://git@github.com/org/repo.git", filepath.Join(cache, "github.com", "org", "repo")},
	} {
		got, err := RemoteClonePath(cache, tc.url)
		if err != nil {
			t.Errorf("RemoteClonePath(%q): %v", tc.url, err)
			continue
		}
		if got != tc.want {
			t.Errorf("RemoteClonePath(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestRemoteClonePathRejectsMalformedSCPURL(t *testing.T) {
	if _, err := RemoteClonePath("/cache", "git@github.com"); err == nil {
		t.Fatal("RemoteClonePath succeeded for a git@ URL with no colon")
	}
}

func TestEffectiveCacheDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &Config{}
	got, err := cfg.EffectiveCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".local", "share", "gitgrove", "repos")
	if got != want {
		t.Errorf("default cache dir = %q, want %q", got, want)
	}

	cfg.CacheDir = "/somewhere/else"
	if got, _ := cfg.EffectiveCacheDir(); got != "/somewhere/else" {
		t.Errorf("configured cache dir = %q, want /somewhere/else", got)
	}
}

func TestDirEffectiveMode(t *testing.T) {
	if got := (Dir{}).EffectiveMode(); got != dirs.ModeCopy {
		t.Errorf("unset mode = %q, want %q — a hand-written config may omit it", got, dirs.ModeCopy)
	}
	if got := (Dir{Mode: dirs.ModeSymlink}).EffectiveMode(); got != dirs.ModeSymlink {
		t.Errorf("explicit mode = %q, want %q", got, dirs.ModeSymlink)
	}
}

func TestLoadMissingFileReturnsEmptyConfig(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "absent.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Groves == nil {
		t.Error("Groves map is nil; callers index it directly")
	}
	if len(cfg.Groves) != 0 {
		t.Errorf("Groves = %v, want empty", cfg.Groves)
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("this is not = = toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load succeeded on malformed TOML")
	}
}

func TestLoadExpandsHomePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `
cache_dir = "~/cache"

[groves.demo]
path = "~/groves/demo"

[[groves.demo.repos]]
name = "backend"
source = "~/code/backend"
branch = "main"

[[groves.demo.dirs]]
name = "docs"
source = "~/notes/docs"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	g := cfg.Groves["demo"]
	if g == nil {
		t.Fatal("grove demo missing")
	}
	for _, tc := range []struct{ got, want string }{
		{cfg.CacheDir, filepath.Join(home, "cache")},
		{g.Path, filepath.Join(home, "groves", "demo")},
		{g.Repos[0].Source, filepath.Join(home, "code", "backend")},
		{g.Dirs[0].Source, filepath.Join(home, "notes", "docs")},
	} {
		if tc.got != tc.want {
			t.Errorf("path = %q, want %q", tc.got, tc.want)
		}
	}
}

// A config written as "~/code/backend" must survive a load/save round trip
// rather than being rewritten absolute.
func TestSaveContractsHomePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(t.TempDir(), "config.toml")

	cfg := &Config{
		CacheDir: filepath.Join(home, "cache"),
		Groves: map[string]*Grove{
			"demo": {
				Path:  filepath.Join(home, "groves", "demo"),
				Repos: []Repo{{Name: "backend", Source: filepath.Join(home, "code", "backend"), Branch: "main"}},
				Dirs:  []Dir{{Name: "docs", Source: filepath.Join(home, "notes", "docs"), Mode: dirs.ModeCopy}},
			},
		},
	}
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{`"~/cache"`, `"~/groves/demo"`, `"~/code/backend"`, `"~/notes/docs"`} {
		if !strings.Contains(text, want) {
			t.Errorf("config does not contain %s:\n%s", want, text)
		}
	}
	if strings.Contains(text, home+"/") {
		t.Errorf("config still holds absolute home paths:\n%s", text)
	}

	// The in-memory config stays expanded, so callers that keep using it after
	// a save still see absolute paths.
	if cfg.Groves["demo"].Dirs[0].Source != filepath.Join(home, "notes", "docs") {
		t.Errorf("Save mutated the in-memory config")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(t.TempDir(), "config.toml")

	want := &Config{
		Groves: map[string]*Grove{
			"demo": {
				Path: filepath.Join(home, "groves", "demo"),
				Repos: []Repo{
					{Name: "backend", Source: filepath.Join(home, "code", "backend"), Branch: "feat/x"},
					{Name: "frontend", Source: "https://github.com/org/frontend", Branch: "main"},
				},
				Dirs: []Dir{
					{Name: "docs", Source: filepath.Join(home, "notes", "docs"), Mode: dirs.ModeCopy},
					{Name: "ex", Source: "/opt/examples", Mode: dirs.ModeSymlink},
				},
			},
		},
	}
	if err := Save(want, path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	g := got.Groves["demo"]
	if g == nil {
		t.Fatal("grove demo missing after round trip")
	}
	if len(g.Repos) != 2 || len(g.Dirs) != 2 {
		t.Fatalf("got %d repos and %d dirs, want 2 and 2", len(g.Repos), len(g.Dirs))
	}
	for i, w := range want.Groves["demo"].Repos {
		if g.Repos[i] != w {
			t.Errorf("repo %d = %+v, want %+v", i, g.Repos[i], w)
		}
	}
	for i, w := range want.Groves["demo"].Dirs {
		if g.Dirs[i] != w {
			t.Errorf("dir %d = %+v, want %+v", i, g.Dirs[i], w)
		}
	}
}

// A remote source is not a filesystem path, so contraction must leave it alone.
func TestSaveLeavesRemoteSourcesAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(t.TempDir(), "config.toml")

	cfg := &Config{Groves: map[string]*Grove{
		"demo": {Path: "/groves/demo", Repos: []Repo{
			{Name: "frontend", Source: "git@github.com:org/frontend.git", Branch: "main"},
		}},
	}}
	if err := Save(cfg, path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if src := got.Groves["demo"].Repos[0].Source; src != "git@github.com:org/frontend.git" {
		t.Errorf("source = %q, want it unchanged", src)
	}
}

// Groves with no plain directories keep their original config shape.
func TestSaveOmitsEmptyDirs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "config.toml")

	cfg := &Config{Groves: map[string]*Grove{"demo": {Path: "/groves/demo"}}}
	if err := Save(cfg, path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "dirs") {
		t.Errorf("empty dirs list was written:\n%s", raw)
	}
}

func TestSaveCreatesDirectoryAndSetsPermissions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "nested", "deeper", "config.toml")

	if err := Save(&Config{Groves: map[string]*Grove{}}, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o644 {
		t.Errorf("config mode = %o, want 644", got)
	}
}

// The write is atomic, so an interrupted save cannot leave a truncated config
// behind — and no temporary files may be left lying around either.
func TestSaveLeavesNoTempFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	for i := 0; i < 3; i++ {
		if err := Save(&Config{Groves: map[string]*Grove{"demo": {Path: "/groves/demo"}}}, path); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.toml" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory holds %v, want only config.toml", names)
	}
}

func TestDefaultPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".config", "gitgrove", "config.toml"); got != want {
		t.Errorf("DefaultPath = %q, want %q", got, want)
	}
}

// --- error paths -------------------------------------------------------

// A grove path that is exactly the home directory contracts to a bare "~".
func TestSaveContractsBareHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(t.TempDir(), "config.toml")

	cfg := &Config{CacheDir: home, Groves: map[string]*Grove{"demo": {Path: home}}}
	if err := Save(cfg, path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `= "~"`) {
		t.Errorf("home was not contracted to ~:\n%s", raw)
	}
}

func TestRemoteClonePathRejectsUnparsableURL(t *testing.T) {
	if _, err := RemoteClonePath("/cache", "https://exa\x7fmple.com/org/repo"); err == nil {
		t.Fatal("RemoteClonePath succeeded for an unparsable URL")
	}
}

// Without a home directory the paths that depend on one must report why,
// rather than silently writing somewhere unexpected.
func TestHomeDirectoryFailures(t *testing.T) {
	t.Setenv("HOME", "")

	if _, err := DefaultPath(); err == nil {
		t.Error("DefaultPath succeeded with no home directory")
	}
	if _, err := (&Config{}).EffectiveCacheDir(); err == nil {
		t.Error("EffectiveCacheDir succeeded with no home directory")
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[groves.demo]\npath = \"~/groves/demo\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("Load succeeded with no home directory to expand ~ against")
	}
}

func TestSaveReportsAnUnwritableDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	err := Save(&Config{Groves: map[string]*Grove{}}, filepath.Join(dir, "config.toml"))
	if err == nil {
		t.Fatal("Save succeeded into a read-only directory")
	}
	if !strings.Contains(err.Error(), "config") {
		t.Errorf("error = %v, want it to say what failed", err)
	}
}
