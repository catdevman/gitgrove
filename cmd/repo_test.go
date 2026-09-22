package cmd

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/catdevman/gitgrove/internal/config"
	"github.com/catdevman/gitgrove/internal/dirs"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// --- harness -----------------------------------------------------------

// resetFlags returns every flag to its default. The commands are package-level
// singletons, so without this a --symlink in one test would still be set in the
// next.
func resetFlags(c *cobra.Command) {
	reset := func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	}
	c.Flags().VisitAll(reset)
	c.PersistentFlags().VisitAll(reset)
	for _, sub := range c.Commands() {
		resetFlags(sub)
	}
}

// runCLI executes gitgrove as a user would, and returns what it printed.
func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	resetFlags(rootCmd)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w

	rootCmd.SetArgs(args)
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	runErr := rootCmd.Execute()

	os.Stdout = saved
	w.Close()
	out, _ := io.ReadAll(r)
	r.Close()
	return string(out), runErr
}

// testEnv sets up an isolated home and config path, and returns the config path.
func testEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_AUTHOR_NAME", "gitgrove test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "gitgrove test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.invalid")
	return filepath.Join(t.TempDir(), "config.toml")
}

func loadConfigFile(t *testing.T, path string) *config.Config {
	t.Helper()
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}

func initRepo(t *testing.T, at string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	if err := os.MkdirAll(at, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		out, err := exec.Command("git", append([]string{"-C", at}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(at, "file.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "initial")
	return at
}

func makeDir(t *testing.T, at string, files ...string) string {
	t.Helper()
	if err := os.MkdirAll(at, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(at, f), []byte(f+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return at
}

// --- argument parsing --------------------------------------------------

func TestParseAddArg(t *testing.T) {
	for _, tc := range []struct {
		name       string
		arg        string
		asDir      bool
		wantSource string
		wantBranch string
		wantErr    bool
	}{
		{name: "local repo", arg: "/code/backend:feat/api", wantSource: "/code/backend", wantBranch: "feat/api"},
		{name: "branch with slashes", arg: "~/code/backend:feat/a/b", wantSource: "~/code/backend", wantBranch: "feat/a/b"},
		{name: "plain directory", arg: "/notes/docs", wantSource: "/notes/docs"},
		{name: "relative directory", arg: "./docs", wantSource: "./docs"},

		{name: "https with branch", arg: "https://github.com/org/repo:main", wantSource: "https://github.com/org/repo", wantBranch: "main"},
		{name: "https .git with branch", arg: "https://github.com/org/repo.git:feat/x", wantSource: "https://github.com/org/repo.git", wantBranch: "feat/x"},
		{name: "scp with branch", arg: "git@github.com:org/repo.git:main", wantSource: "git@github.com:org/repo.git", wantBranch: "main"},
		{name: "ssh with branch", arg: "ssh://git@github.com/org/repo:main", wantSource: "ssh://git@github.com/org/repo", wantBranch: "main"},

		// A remote with no branch used to split on the scheme colon and yield
		// a source of "https" — it must be rejected instead.
		{name: "https without branch", arg: "https://github.com/org/repo", wantErr: true},
		{name: "scp without branch", arg: "git@github.com:org/repo.git", wantErr: true},
		{name: "ssh without branch", arg: "ssh://git@github.com/org/repo", wantErr: true},

		{name: "empty branch", arg: "/code/backend:", wantErr: true},
		{name: "empty source", arg: ":main", wantErr: true},

		// --dir takes the argument literally, colons and all.
		{name: "forced dir keeps colon", arg: "/notes/weird:name", asDir: true, wantSource: "/notes/weird:name"},
		{name: "forced dir on a URL", arg: "https://example.com/x", asDir: true, wantSource: "https://example.com/x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source, branch, err := parseAddArg(tc.arg, tc.asDir)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseAddArg(%q) = (%q, %q), want an error", tc.arg, source, branch)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAddArg(%q): %v", tc.arg, err)
			}
			if source != tc.wantSource || branch != tc.wantBranch {
				t.Errorf("parseAddArg(%q) = (%q, %q), want (%q, %q)",
					tc.arg, source, branch, tc.wantSource, tc.wantBranch)
			}
		})
	}
}

func TestCheckDirSource(t *testing.T) {
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "backend"))
	plain := makeDir(t, filepath.Join(tmp, "docs"))

	if err := checkDirSource(plain, false); err != nil {
		t.Errorf("plain directory rejected: %v", err)
	}
	err := checkDirSource(repo, false)
	if err == nil {
		t.Fatal("a git repo was accepted as a plain directory")
	}
	if !strings.Contains(err.Error(), "--dir") {
		t.Errorf("error = %v, want it to mention the --dir escape hatch", err)
	}
	if err := checkDirSource(repo, true); err != nil {
		t.Errorf("--dir did not override the check: %v", err)
	}
}

// --- add ---------------------------------------------------------------

func TestAddRepoAndDirectory(t *testing.T) {
	cfgFile := testEnv(t)
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "backend"))
	docs := makeDir(t, filepath.Join(tmp, "docs"), "README.md")

	if _, err := runCLI(t, "--config", cfgFile, "create", "demo", "--path", filepath.Join(tmp, "grove")); err != nil {
		t.Fatal(err)
	}
	out, err := runCLI(t, "--config", cfgFile, "add", "demo", repo+":feat/x", docs)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.Contains(out, `added repo "backend"`) || !strings.Contains(out, `added directory "docs" (copy)`) {
		t.Errorf("output did not report both entries:\n%s", out)
	}

	g := loadConfigFile(t, cfgFile).Groves["demo"]
	if len(g.Repos) != 1 || g.Repos[0].Branch != "feat/x" || g.Repos[0].Source != repo {
		t.Errorf("repos = %+v", g.Repos)
	}
	if len(g.Dirs) != 1 || g.Dirs[0].Name != "docs" || g.Dirs[0].Mode != dirs.ModeCopy {
		t.Errorf("dirs = %+v, want one copy-mode entry", g.Dirs)
	}
}

func TestAddDefaultsToCopyAndHonoursSymlink(t *testing.T) {
	cfgFile := testEnv(t)
	tmp := t.TempDir()
	docs := makeDir(t, filepath.Join(tmp, "docs"))
	examples := makeDir(t, filepath.Join(tmp, "examples"))

	runCLI(t, "--config", cfgFile, "create", "demo", "--path", filepath.Join(tmp, "grove"))
	if _, err := runCLI(t, "--config", cfgFile, "add", "demo", docs); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "--config", cfgFile, "add", "demo", examples, "--symlink"); err != nil {
		t.Fatal(err)
	}

	g := loadConfigFile(t, cfgFile).Groves["demo"]
	if g.Dirs[0].Mode != dirs.ModeCopy {
		t.Errorf("default mode = %q, want copy", g.Dirs[0].Mode)
	}
	if g.Dirs[1].Mode != dirs.ModeSymlink {
		t.Errorf("--symlink mode = %q, want symlink", g.Dirs[1].Mode)
	}
}

func TestAddNameFlag(t *testing.T) {
	cfgFile := testEnv(t)
	tmp := t.TempDir()
	docs := makeDir(t, filepath.Join(tmp, "api-docs"))

	runCLI(t, "--config", cfgFile, "create", "demo", "--path", filepath.Join(tmp, "grove"))
	if _, err := runCLI(t, "--config", cfgFile, "add", "demo", docs, "--name", "docs"); err != nil {
		t.Fatal(err)
	}
	if got := loadConfigFile(t, cfgFile).Groves["demo"].Dirs[0].Name; got != "docs" {
		t.Errorf("name = %q, want docs", got)
	}
}

func TestAddNameFlagRejectsMultipleSources(t *testing.T) {
	cfgFile := testEnv(t)
	tmp := t.TempDir()
	a := makeDir(t, filepath.Join(tmp, "a"))
	b := makeDir(t, filepath.Join(tmp, "b"))

	runCLI(t, "--config", cfgFile, "create", "demo", "--path", filepath.Join(tmp, "grove"))
	if _, err := runCLI(t, "--config", cfgFile, "add", "demo", a, b, "--name", "x"); err == nil {
		t.Fatal("add succeeded with --name and two sources")
	}
}

func TestAddRejectsGitRepoWithoutBranch(t *testing.T) {
	cfgFile := testEnv(t)
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "backend"))

	runCLI(t, "--config", cfgFile, "create", "demo", "--path", filepath.Join(tmp, "grove"))
	_, err := runCLI(t, "--config", cfgFile, "add", "demo", repo)
	if err == nil {
		t.Fatal("add succeeded for a git repo with no branch")
	}
	if g := loadConfigFile(t, cfgFile).Groves["demo"]; len(g.Dirs) != 0 {
		t.Errorf("config was modified anyway: %+v", g.Dirs)
	}
}

func TestAddDirFlagCopiesAGitRepo(t *testing.T) {
	cfgFile := testEnv(t)
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "backend"))

	runCLI(t, "--config", cfgFile, "create", "demo", "--path", filepath.Join(tmp, "grove"))
	if _, err := runCLI(t, "--config", cfgFile, "add", "demo", repo, "--dir"); err != nil {
		t.Fatalf("add --dir: %v", err)
	}
	if got := loadConfigFile(t, cfgFile).Groves["demo"].Dirs; len(got) != 1 {
		t.Errorf("dirs = %+v, want the repo added as a plain directory", got)
	}
}

func TestAddSymlinkWithoutADirectoryIsAnError(t *testing.T) {
	cfgFile := testEnv(t)
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "backend"))

	runCLI(t, "--config", cfgFile, "create", "demo", "--path", filepath.Join(tmp, "grove"))
	if _, err := runCLI(t, "--config", cfgFile, "add", "demo", repo+":main", "--symlink"); err == nil {
		t.Fatal("add succeeded with --symlink and only repos")
	}
}

func TestAddRejectsDuplicateNames(t *testing.T) {
	cfgFile := testEnv(t)
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "shared"))
	docs := makeDir(t, filepath.Join(tmp, "other", "shared"))

	runCLI(t, "--config", cfgFile, "create", "demo", "--path", filepath.Join(tmp, "grove"))
	if _, err := runCLI(t, "--config", cfgFile, "add", "demo", repo+":main"); err != nil {
		t.Fatal(err)
	}

	// A directory whose basename collides with an existing repo shares the
	// same destination, so it has to be rejected.
	if _, err := runCLI(t, "--config", cfgFile, "add", "demo", docs); err == nil {
		t.Fatal("add succeeded for a directory colliding with a repo name")
	}
	// ... and within a single invocation too.
	if _, err := runCLI(t, "--config", cfgFile, "add", "demo", docs, docs); err == nil {
		t.Fatal("add succeeded for the same directory twice")
	}
}

func TestAddUnknownGrove(t *testing.T) {
	cfgFile := testEnv(t)
	docs := makeDir(t, filepath.Join(t.TempDir(), "docs"))
	if _, err := runCLI(t, "--config", cfgFile, "add", "nope", docs); err == nil {
		t.Fatal("add succeeded for a grove that does not exist")
	}
}

// --- remove ------------------------------------------------------------

func TestRemoveRepoAndDirectory(t *testing.T) {
	cfgFile := testEnv(t)
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "backend"))
	docs := makeDir(t, filepath.Join(tmp, "docs"), "README.md")

	runCLI(t, "--config", cfgFile, "create", "demo", "--path", filepath.Join(tmp, "grove"))
	runCLI(t, "--config", cfgFile, "add", "demo", repo+":feat/x", docs)

	out, err := runCLI(t, "--config", cfgFile, "remove", "demo", "docs")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !strings.Contains(out, `removed directory "docs"`) {
		t.Errorf("output = %q, want it to name the directory", out)
	}
	g := loadConfigFile(t, cfgFile).Groves["demo"]
	if len(g.Dirs) != 0 || len(g.Repos) != 1 {
		t.Errorf("after removing the directory: repos=%+v dirs=%+v", g.Repos, g.Dirs)
	}

	if _, err := runCLI(t, "--config", cfgFile, "remove", "demo", "backend"); err != nil {
		t.Fatalf("remove repo: %v", err)
	}
	if got := loadConfigFile(t, cfgFile).Groves["demo"].Repos; len(got) != 0 {
		t.Errorf("repos = %+v, want empty", got)
	}
}

func TestRemovePrunesFromDisk(t *testing.T) {
	cfgFile := testEnv(t)
	tmp := t.TempDir()
	grovePath := filepath.Join(tmp, "grove")
	repo := initRepo(t, filepath.Join(tmp, "backend"))
	docs := makeDir(t, filepath.Join(tmp, "docs"), "README.md")
	examples := makeDir(t, filepath.Join(tmp, "examples"), "e.txt")

	runCLI(t, "--config", cfgFile, "create", "demo", "--path", grovePath)
	runCLI(t, "--config", cfgFile, "add", "demo", repo+":feat/x", docs)
	runCLI(t, "--config", cfgFile, "add", "demo", examples, "--symlink")
	if _, err := runCLI(t, "--config", cfgFile, "sync", "demo"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if _, err := runCLI(t, "--config", cfgFile, "remove", "demo", "docs", "--prune"); err != nil {
		t.Fatalf("remove --prune: %v", err)
	}
	if _, err := os.Stat(filepath.Join(grovePath, "docs")); !os.IsNotExist(err) {
		t.Errorf("copied directory was not pruned: %v", err)
	}
	if _, err := os.Stat(filepath.Join(docs, "README.md")); err != nil {
		t.Errorf("source directory was damaged: %v", err)
	}

	if _, err := runCLI(t, "--config", cfgFile, "remove", "demo", "examples", "--prune"); err != nil {
		t.Fatalf("remove symlink --prune: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(grovePath, "examples")); !os.IsNotExist(err) {
		t.Errorf("symlink was not pruned")
	}
	if _, err := os.Stat(filepath.Join(examples, "e.txt")); err != nil {
		t.Errorf("symlink source was damaged: %v", err)
	}
}

func TestRemoveUnknownEntry(t *testing.T) {
	cfgFile := testEnv(t)
	runCLI(t, "--config", cfgFile, "create", "demo", "--path", filepath.Join(t.TempDir(), "grove"))
	if _, err := runCLI(t, "--config", cfgFile, "remove", "demo", "nothing"); err == nil {
		t.Fatal("remove succeeded for an entry that does not exist")
	}
}
