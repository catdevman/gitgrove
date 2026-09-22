package grove

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/catdevman/gitgrove/internal/config"
	"github.com/catdevman/gitgrove/internal/dirs"
	"github.com/catdevman/gitgrove/internal/git"
)

func isolate(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_AUTHOR_NAME", "gitgrove test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "gitgrove test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.invalid")
}

func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func initRepo(t *testing.T, at string) string {
	t.Helper()
	if err := os.MkdirAll(at, 0o755); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, at, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(at, "file.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, at, "add", "-A")
	gitCmd(t, at, "commit", "-qm", "initial")
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

// findIssue returns the first issue for a repo/check pair.
func findIssue(issues []Issue, repo, check string) (Issue, bool) {
	for _, i := range issues {
		if i.Repo == repo && i.Check == check {
			return i, true
		}
	}
	return Issue{}, false
}

func TestDoctorHealthyGrove(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "backend"))
	docs := makeDir(t, filepath.Join(tmp, "docs"), "README.md")

	g := &config.Grove{
		Path:  filepath.Join(tmp, "grove"),
		Repos: []config.Repo{{Name: "backend", Source: repo, Branch: "main"}},
		Dirs:  []config.Dir{{Name: "docs", Source: docs}},
	}
	for _, issue := range Doctor("demo", g, filepath.Join(tmp, "cache")) {
		if issue.Severity != SeverityOK {
			t.Errorf("unexpected finding: %+v", issue)
		}
	}
}

func TestDoctorNonGitSourcePointsAtPlainDirectories(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	plain := makeDir(t, filepath.Join(tmp, "notes"))

	g := &config.Grove{
		Path:  filepath.Join(tmp, "grove"),
		Repos: []config.Repo{{Name: "notes", Source: plain, Branch: "main"}},
	}
	issue, ok := findIssue(Doctor("demo", g, tmp), "notes", "source")
	if !ok {
		t.Fatal("no source finding")
	}
	if issue.Severity != SeverityError {
		t.Errorf("severity = %v, want error", issue.Severity)
	}
	if !strings.Contains(issue.Message, "without a branch") {
		t.Errorf("message = %q, want it to suggest adding the path without a branch", issue.Message)
	}
}

func TestDoctorMissingBranchIsOnlyAWarning(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "backend"))

	g := &config.Grove{
		Path:  filepath.Join(tmp, "grove"),
		Repos: []config.Repo{{Name: "backend", Source: repo, Branch: "feat/not-yet"}},
	}
	issue, ok := findIssue(Doctor("demo", g, tmp), "backend", "branch")
	if !ok {
		t.Fatal("no branch finding")
	}
	if issue.Severity != SeverityWarn {
		t.Errorf("severity = %v, want warn — sync creates the branch", issue.Severity)
	}
}

func TestDoctorRemoteSourceReportsClonePath(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	cache := filepath.Join(tmp, "cache")

	g := &config.Grove{
		Path:  filepath.Join(tmp, "grove"),
		Repos: []config.Repo{{Name: "frontend", Source: "https://github.com/org/frontend", Branch: "main"}},
	}
	issues := Doctor("demo", g, cache)
	issue, ok := findIssue(issues, "frontend", "source")
	if !ok {
		t.Fatal("no source finding")
	}
	if issue.Severity != SeverityOK || !strings.Contains(issue.Message, filepath.Join(cache, "github.com", "org", "frontend")) {
		t.Errorf("finding = %+v, want an OK naming the clone path", issue)
	}
	// A remote's branch cannot be checked before the clone exists.
	if _, ok := findIssue(issues, "frontend", "branch"); ok {
		t.Error("branch was checked for a remote that has not been cloned")
	}
}

func TestDoctorDirSourceProblems(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	file := filepath.Join(tmp, "a-file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		dir  config.Dir
		want string
	}{
		{"missing", config.Dir{Name: "gone", Source: filepath.Join(tmp, "absent")}, "does not exist"},
		{"not a directory", config.Dir{Name: "file", Source: file}, "not a directory"},
		{"bad mode", config.Dir{Name: "weird", Source: tmp, Mode: "move"}, "unknown mode"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &config.Grove{Path: filepath.Join(tmp, "grove"), Dirs: []config.Dir{tc.dir}}
			issues := Doctor("demo", g, tmp)
			if len(issues) == 0 {
				t.Fatal("no findings")
			}
			if issues[0].Severity != SeverityError {
				t.Errorf("severity = %v, want error", issues[0].Severity)
			}
			if !strings.Contains(issues[0].Message, tc.want) {
				t.Errorf("message = %q, want it to mention %q", issues[0].Message, tc.want)
			}
		})
	}
}

func TestDoctorWarnsWhenADirIsActuallyARepo(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "backend"))

	g := &config.Grove{
		Path: filepath.Join(tmp, "grove"),
		Dirs: []config.Dir{{Name: "backend", Source: repo}},
	}
	var warned bool
	for _, issue := range Doctor("demo", g, tmp) {
		if issue.Severity == SeverityWarn && strings.Contains(issue.Message, "git repo") {
			warned = true
		}
	}
	if !warned {
		t.Error("no warning that the directory is a git repo")
	}
}

// Repos and directories share the grove directory, so a name used twice is an
// error whichever lists it comes from.
func TestDoctorDetectsDuplicateDestinations(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "backend"))
	docs := makeDir(t, filepath.Join(tmp, "docs"))

	t.Run("two repos", func(t *testing.T) {
		g := &config.Grove{Path: filepath.Join(tmp, "grove"), Repos: []config.Repo{
			{Name: "backend", Source: repo, Branch: "main"},
			{Name: "backend", Source: repo, Branch: "main"},
		}}
		if _, ok := findIssue(Doctor("demo", g, tmp), "backend", "dest"); !ok {
			t.Error("duplicate worktree path not reported")
		}
	})

	t.Run("repo and dir", func(t *testing.T) {
		g := &config.Grove{
			Path:  filepath.Join(tmp, "grove"),
			Repos: []config.Repo{{Name: "shared", Source: repo, Branch: "main"}},
			Dirs:  []config.Dir{{Name: "shared", Source: docs}},
		}
		if _, ok := findIssue(Doctor("demo", g, tmp), "shared", "dest"); !ok {
			t.Error("a directory colliding with a repo was not reported")
		}
	})
}

func TestDoctorGrovePathChecks(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	file := filepath.Join(tmp, "a-file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A path that does not exist yet is fine: sync creates it.
	g := &config.Grove{Path: filepath.Join(tmp, "not", "yet", "there")}
	if issue, ok := findIssue(Doctor("demo", g, tmp), "", "grove-path"); ok {
		t.Errorf("unexpected finding for a creatable path: %+v", issue)
	}

	g = &config.Grove{Path: file}
	issue, ok := findIssue(Doctor("demo", g, tmp), "", "grove-path")
	if !ok {
		t.Fatal("a file in the grove path's place was not reported")
	}
	if !strings.Contains(issue.Message, "not a directory") {
		t.Errorf("message = %q", issue.Message)
	}
}

func TestSyncCreatesWorktreesAndDirectories(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "backend"))
	docs := makeDir(t, filepath.Join(tmp, "docs"), "README.md")
	examples := makeDir(t, filepath.Join(tmp, "examples"), "e.txt")
	grovePath := filepath.Join(tmp, "grove")

	g := &config.Grove{
		Path:  grovePath,
		Repos: []config.Repo{{Name: "backend", Source: repo, Branch: "feat/x"}},
		Dirs: []config.Dir{
			{Name: "docs", Source: docs, Mode: dirs.ModeCopy},
			{Name: "ex", Source: examples, Mode: dirs.ModeSymlink},
		},
	}
	if err := Sync("demo", g, filepath.Join(tmp, "cache")); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if got, _ := git.CurrentBranch(filepath.Join(grovePath, "backend")); got != "feat/x" {
		t.Errorf("worktree branch = %q, want feat/x", got)
	}
	if _, err := os.Stat(filepath.Join(grovePath, "docs", "README.md")); err != nil {
		t.Errorf("directory was not copied: %v", err)
	}
	fi, err := os.Lstat(filepath.Join(grovePath, "ex"))
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink-mode directory was copied instead of linked")
	}
}

func TestSyncIsIdempotent(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "backend"))
	docs := makeDir(t, filepath.Join(tmp, "docs"), "README.md")
	grovePath := filepath.Join(tmp, "grove")

	g := &config.Grove{
		Path:  grovePath,
		Repos: []config.Repo{{Name: "backend", Source: repo, Branch: "feat/x"}},
		Dirs:  []config.Dir{{Name: "docs", Source: docs}},
	}
	for i := 0; i < 2; i++ {
		if err := Sync("demo", g, filepath.Join(tmp, "cache")); err != nil {
			t.Fatalf("Sync %d: %v", i, err)
		}
	}
}

func TestSyncFixesBranchDrift(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "backend"))
	grovePath := filepath.Join(tmp, "grove")

	g := &config.Grove{
		Path:  grovePath,
		Repos: []config.Repo{{Name: "backend", Source: repo, Branch: "feat/one"}},
	}
	if err := Sync("demo", g, tmp); err != nil {
		t.Fatal(err)
	}

	g.Repos[0].Branch = "feat/two"
	if err := Sync("demo", g, tmp); err != nil {
		t.Fatalf("Sync after drift: %v", err)
	}
	if got, _ := git.CurrentBranch(filepath.Join(grovePath, "backend")); got != "feat/two" {
		t.Errorf("branch = %q, want feat/two", got)
	}
}

func TestSyncRetargetsSymlinkWhenSourceChanges(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	first := makeDir(t, filepath.Join(tmp, "first"))
	second := makeDir(t, filepath.Join(tmp, "second"))
	grovePath := filepath.Join(tmp, "grove")

	g := &config.Grove{
		Path: grovePath,
		Dirs: []config.Dir{{Name: "ref", Source: first, Mode: dirs.ModeSymlink}},
	}
	if err := Sync("demo", g, tmp); err != nil {
		t.Fatal(err)
	}
	g.Dirs[0].Source = second
	if err := Sync("demo", g, tmp); err != nil {
		t.Fatalf("Sync after source change: %v", err)
	}

	target, err := os.Readlink(filepath.Join(grovePath, "ref"))
	if err != nil {
		t.Fatal(err)
	}
	if target != second {
		t.Errorf("symlink target = %q, want %q", target, second)
	}
}

func TestSyncDoesNotClobberACopy(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	docs := makeDir(t, filepath.Join(tmp, "docs"), "README.md")
	grovePath := filepath.Join(tmp, "grove")

	g := &config.Grove{Path: grovePath, Dirs: []config.Dir{{Name: "docs", Source: docs}}}
	if err := Sync("demo", g, tmp); err != nil {
		t.Fatal(err)
	}
	edited := filepath.Join(grovePath, "docs", "README.md")
	if err := os.WriteFile(edited, []byte("work in progress\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Sync("demo", g, tmp); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(edited)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "work in progress\n" {
		t.Errorf("copy was overwritten by sync: %q", got)
	}
}

func TestSyncReportsGroveAndEntryInErrors(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	g := &config.Grove{
		Path: filepath.Join(tmp, "grove"),
		Dirs: []config.Dir{{Name: "missing", Source: filepath.Join(tmp, "absent")}},
	}
	err := Sync("demo", g, tmp)
	if err == nil {
		t.Fatal("Sync succeeded with a missing source")
	}
	if !strings.Contains(err.Error(), "demo") || !strings.Contains(err.Error(), "missing") {
		t.Errorf("error = %v, want it to name the grove and the entry", err)
	}
}

func TestSyncFailsLoudlyWhenARemoteCannotBeCloned(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	origin := initRepo(t, filepath.Join(tmp, "origin"))
	cache := filepath.Join(tmp, "cache")
	grovePath := filepath.Join(tmp, "grove")

	// An unreachable remote must fail loudly rather than silently producing an
	// empty worktree. (The successful clone path is covered by
	// TestRemoveUsesTheCachedCloneForRemotes, which seeds the cache.)
	g := &config.Grove{
		Path:  grovePath,
		Repos: []config.Repo{{Name: "origin", Source: "ssh://example.invalid" + origin, Branch: "feat/x"}},
	}
	if err := Sync("demo", g, cache); err == nil {
		t.Fatal("Sync succeeded despite an unreachable remote")
	}
	if _, err := os.Stat(filepath.Join(grovePath, "origin")); !os.IsNotExist(err) {
		t.Error("a worktree was created for a repo that could not be cloned")
	}
}

func TestGetStatus(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "backend"))
	docs := makeDir(t, filepath.Join(tmp, "docs"))
	examples := makeDir(t, filepath.Join(tmp, "examples"))
	grovePath := filepath.Join(tmp, "grove")

	g := &config.Grove{
		Path: grovePath,
		Repos: []config.Repo{
			{Name: "backend", Source: repo, Branch: "feat/x"},
			{Name: "absent", Source: repo, Branch: "main"},
		},
		Dirs: []config.Dir{
			{Name: "docs", Source: docs},
			{Name: "ex", Source: examples, Mode: dirs.ModeSymlink},
			{Name: "later", Source: docs},
		},
	}
	// Sync everything except the entries meant to stay missing.
	partial := &config.Grove{
		Path:  grovePath,
		Repos: g.Repos[:1],
		Dirs:  g.Dirs[:2],
	}
	if err := Sync("demo", partial, tmp); err != nil {
		t.Fatal(err)
	}

	s := GetStatus("demo", g)
	if s.GroveName != "demo" || s.GrovePath != grovePath {
		t.Errorf("status header = %+v", s)
	}
	if len(s.Repos) != 2 || len(s.Dirs) != 3 {
		t.Fatalf("got %d repos and %d dirs, want 2 and 3", len(s.Repos), len(s.Dirs))
	}

	if !s.Repos[0].Present || !s.Repos[0].BranchMatches || s.Repos[0].ActualBranch != "feat/x" {
		t.Errorf("synced repo status = %+v", s.Repos[0])
	}
	if s.Repos[1].Present {
		t.Errorf("unsynced repo reported present: %+v", s.Repos[1])
	}
	if s.Dirs[0].State != DirOK || s.Dirs[1].State != DirOK {
		t.Errorf("synced dirs = %+v, %+v, want both OK", s.Dirs[0], s.Dirs[1])
	}
	if s.Dirs[2].State != DirMissing {
		t.Errorf("unsynced dir = %+v, want missing", s.Dirs[2])
	}
}

func TestGetStatusReportsBranchMismatch(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "backend"))
	grovePath := filepath.Join(tmp, "grove")

	g := &config.Grove{
		Path:  grovePath,
		Repos: []config.Repo{{Name: "backend", Source: repo, Branch: "feat/one"}},
	}
	if err := Sync("demo", g, tmp); err != nil {
		t.Fatal(err)
	}
	g.Repos[0].Branch = "feat/two"

	rs := GetStatus("demo", g).Repos[0]
	if !rs.Present || rs.BranchMatches {
		t.Errorf("status = %+v, want present with a branch mismatch", rs)
	}
	if rs.ActualBranch != "feat/one" {
		t.Errorf("ActualBranch = %q, want feat/one", rs.ActualBranch)
	}
}

func TestGetStatusDirDriftAndConflict(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	first := makeDir(t, filepath.Join(tmp, "first"))
	second := makeDir(t, filepath.Join(tmp, "second"))
	grovePath := filepath.Join(tmp, "grove")
	if err := os.MkdirAll(grovePath, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("symlink pointing elsewhere", func(t *testing.T) {
		dest := filepath.Join(grovePath, "drifted")
		if err := os.Symlink(first, dest); err != nil {
			t.Fatal(err)
		}
		g := &config.Grove{Path: grovePath, Dirs: []config.Dir{
			{Name: "drifted", Source: second, Mode: dirs.ModeSymlink},
		}}
		ds := GetStatus("demo", g).Dirs[0]
		if ds.State != DirDrifted || ds.Actual != first {
			t.Errorf("status = %+v, want drifted reporting %s", ds, first)
		}
	})

	t.Run("real directory where a symlink is configured", func(t *testing.T) {
		dest := filepath.Join(grovePath, "occupied")
		if err := os.MkdirAll(dest, 0o755); err != nil {
			t.Fatal(err)
		}
		g := &config.Grove{Path: grovePath, Dirs: []config.Dir{
			{Name: "occupied", Source: second, Mode: dirs.ModeSymlink},
		}}
		if ds := GetStatus("demo", g).Dirs[0]; ds.State != DirConflict {
			t.Errorf("state = %v, want conflict", ds.State)
		}
	})

	t.Run("symlink where a copy is configured", func(t *testing.T) {
		dest := filepath.Join(grovePath, "stale-link")
		if err := os.Symlink(first, dest); err != nil {
			t.Fatal(err)
		}
		g := &config.Grove{Path: grovePath, Dirs: []config.Dir{
			{Name: "stale-link", Source: first, Mode: dirs.ModeCopy},
		}}
		if ds := GetStatus("demo", g).Dirs[0]; ds.State != DirDrifted {
			t.Errorf("state = %v, want drifted — the mode changed", ds.State)
		}
	})
}

func TestRemoveTakesDownWorktreesAndDirectories(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "backend"))
	docs := makeDir(t, filepath.Join(tmp, "docs"), "README.md")
	examples := makeDir(t, filepath.Join(tmp, "examples"), "e.txt")
	grovePath := filepath.Join(tmp, "grove")

	g := &config.Grove{
		Path:  grovePath,
		Repos: []config.Repo{{Name: "backend", Source: repo, Branch: "feat/x"}},
		Dirs: []config.Dir{
			{Name: "docs", Source: docs},
			{Name: "ex", Source: examples, Mode: dirs.ModeSymlink},
		},
	}
	if err := Sync("demo", g, tmp); err != nil {
		t.Fatal(err)
	}
	if err := Remove("demo", g, tmp, false); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	for _, name := range []string{"backend", "docs", "ex"} {
		if _, err := os.Lstat(filepath.Join(grovePath, name)); !os.IsNotExist(err) {
			t.Errorf("%s still in the grove: %v", name, err)
		}
	}
	// Sources are never touched, whether copied or linked.
	for _, src := range []string{filepath.Join(repo, "file.txt"), filepath.Join(docs, "README.md"), filepath.Join(examples, "e.txt")} {
		if _, err := os.Stat(src); err != nil {
			t.Errorf("source %s was damaged: %v", src, err)
		}
	}
}

func TestRemoveSkipsEntriesThatAreNotThere(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "backend"))
	docs := makeDir(t, filepath.Join(tmp, "docs"))

	g := &config.Grove{
		Path:  filepath.Join(tmp, "grove"),
		Repos: []config.Repo{{Name: "backend", Source: repo, Branch: "main"}},
		Dirs:  []config.Dir{{Name: "docs", Source: docs}},
	}
	if err := Remove("demo", g, tmp, false); err != nil {
		t.Errorf("Remove on an unsynced grove = %v, want nil", err)
	}
}

// Worktree operations must run against the local repo that owns the worktree —
// for a remote source that is the cached clone, never the URL in the config.
func TestRemoveUsesTheCachedCloneForRemotes(t *testing.T) {
	isolate(t)
	tmp := t.TempDir()
	origin := initRepo(t, filepath.Join(tmp, "origin"))
	cache := filepath.Join(tmp, "cache")
	grovePath := filepath.Join(tmp, "grove")

	clonePath := filepath.Join(cache, "github.com", "org", "repo")
	if err := os.MkdirAll(filepath.Dir(clonePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := git.Clone(origin, clonePath); err != nil {
		t.Fatal(err)
	}

	g := &config.Grove{
		Path:  grovePath,
		Repos: []config.Repo{{Name: "repo", Source: "https://github.com/org/repo", Branch: "feat/x"}},
	}
	if err := Sync("demo", g, cache); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if err := Remove("demo", g, cache, false); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(grovePath, "repo")); !os.IsNotExist(err) {
		t.Error("worktree for a remote-sourced repo was not removed")
	}
}
