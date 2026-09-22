package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// isolate makes git in this test process ignore the developer's own global and
// system config, so results do not depend on whoever runs the suite.
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
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// initRepo creates a repository on branch main with one commit.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-qm", "initial")
	return dir
}

func TestIsGitRepo(t *testing.T) {
	isolate(t)
	repo := initRepo(t)
	if !IsGitRepo(repo) {
		t.Error("IsGitRepo = false for a real repo")
	}
	if IsGitRepo(t.TempDir()) {
		t.Error("IsGitRepo = true for a plain directory")
	}
	if IsGitRepo(filepath.Join(t.TempDir(), "absent")) {
		t.Error("IsGitRepo = true for a path that does not exist")
	}
}

func TestBranchExists(t *testing.T) {
	isolate(t)
	repo := initRepo(t)
	gitCmd(t, repo, "branch", "feature")

	if !BranchExists(repo, "main") {
		t.Error("BranchExists(main) = false")
	}
	if !BranchExists(repo, "feature") {
		t.Error("BranchExists(feature) = false")
	}
	if BranchExists(repo, "nope") {
		t.Error("BranchExists(nope) = true")
	}
}

// A bare name also resolves tags, which would make callers check out a
// detached HEAD instead of the branch they asked for.
func TestBranchExistsIgnoresTags(t *testing.T) {
	isolate(t)
	repo := initRepo(t)
	gitCmd(t, repo, "tag", "v1.0.0")

	if BranchExists(repo, "v1.0.0") {
		t.Error("BranchExists = true for a tag; only refs/heads/ should count")
	}
}

func TestBranchExistsIgnoresRemoteTrackingRefs(t *testing.T) {
	isolate(t)
	origin := initRepo(t)
	gitCmd(t, origin, "branch", "feature")

	clone := filepath.Join(t.TempDir(), "clone")
	if err := Clone(origin, clone); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if BranchExists(clone, "feature") {
		t.Error("BranchExists = true for a remote-tracking branch with no local branch")
	}
}

func TestCurrentBranch(t *testing.T) {
	isolate(t)
	repo := initRepo(t)
	got, err := CurrentBranch(repo)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if got != "main" {
		t.Errorf("CurrentBranch = %q, want main", got)
	}
}

func TestCurrentBranchErrorsOutsideRepo(t *testing.T) {
	isolate(t)
	if _, err := CurrentBranch(t.TempDir()); err == nil {
		t.Error("CurrentBranch succeeded outside a repo")
	}
}

func TestWorktreeAddExistingBranch(t *testing.T) {
	isolate(t)
	repo := initRepo(t)
	gitCmd(t, repo, "branch", "feature")
	dest := filepath.Join(t.TempDir(), "wt")

	if err := WorktreeAdd(repo, dest, "feature"); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	if got, _ := CurrentBranch(dest); got != "feature" {
		t.Errorf("worktree branch = %q, want feature", got)
	}
	if _, err := os.Stat(filepath.Join(dest, "file.txt")); err != nil {
		t.Errorf("worktree has no checkout: %v", err)
	}
}

func TestWorktreeAddCreatesMissingBranchFromHEAD(t *testing.T) {
	isolate(t)
	repo := initRepo(t)
	dest := filepath.Join(t.TempDir(), "wt")

	if err := WorktreeAdd(repo, dest, "feat/new"); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	if got, _ := CurrentBranch(dest); got != "feat/new" {
		t.Errorf("worktree branch = %q, want feat/new", got)
	}
	if !BranchExists(repo, "feat/new") {
		t.Error("branch was not created in the source repo")
	}
}

// A branch that exists only on the remote should be checked out tracking it,
// not created empty from HEAD.
func TestWorktreeAddTracksRemoteBranch(t *testing.T) {
	isolate(t)
	origin := initRepo(t)
	gitCmd(t, origin, "checkout", "-q", "-b", "feature")
	if err := os.WriteFile(filepath.Join(origin, "only-on-feature.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, origin, "add", "-A")
	gitCmd(t, origin, "commit", "-qm", "feature work")
	gitCmd(t, origin, "checkout", "-q", "main")

	clone := filepath.Join(t.TempDir(), "clone")
	if err := Clone(origin, clone); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "wt")
	if err := WorktreeAdd(clone, dest, "feature"); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	if got, _ := CurrentBranch(dest); got != "feature" {
		t.Fatalf("worktree branch = %q, want feature", got)
	}
	if _, err := os.Stat(filepath.Join(dest, "only-on-feature.txt")); err != nil {
		t.Errorf("worktree was created from HEAD instead of tracking origin/feature: %v", err)
	}
	if up := gitCmd(t, dest, "rev-parse", "--abbrev-ref", "feature@{upstream}"); up != "origin/feature" {
		t.Errorf("upstream = %q, want origin/feature", up)
	}
}

func TestRemoteTrackingRefPrefersOrigin(t *testing.T) {
	isolate(t)
	origin := initRepo(t)
	gitCmd(t, origin, "branch", "feature")

	clone := filepath.Join(t.TempDir(), "clone")
	if err := Clone(origin, clone); err != nil {
		t.Fatal(err)
	}
	// A second remote carrying the same branch name must not make the lookup
	// ambiguous: origin wins.
	other := initRepo(t)
	gitCmd(t, other, "branch", "feature")
	gitCmd(t, clone, "remote", "add", "other", other)
	gitCmd(t, clone, "fetch", "-q", "other")

	ref, ok := remoteTrackingRef(clone, "feature")
	if !ok {
		t.Fatal("remoteTrackingRef = not found")
	}
	if ref != "origin/feature" {
		t.Errorf("ref = %q, want origin/feature", ref)
	}
}

func TestRemoteTrackingRefAmbiguousWithoutOrigin(t *testing.T) {
	isolate(t)
	repo := initRepo(t)
	for _, name := range []string{"alpha", "beta"} {
		remote := initRepo(t)
		gitCmd(t, remote, "branch", "feature")
		gitCmd(t, repo, "remote", "add", name, remote)
		gitCmd(t, repo, "fetch", "-q", name)
	}
	if _, ok := remoteTrackingRef(repo, "feature"); ok {
		t.Error("remoteTrackingRef resolved an ambiguous branch; it should decline")
	}
}

func TestRemoteTrackingRefMissing(t *testing.T) {
	isolate(t)
	repo := initRepo(t)
	if _, ok := remoteTrackingRef(repo, "nothing"); ok {
		t.Error("remoteTrackingRef found a branch that does not exist")
	}
}

func TestSwitchBranchExisting(t *testing.T) {
	isolate(t)
	repo := initRepo(t)
	gitCmd(t, repo, "branch", "feature")

	if err := SwitchBranch(repo, "feature"); err != nil {
		t.Fatalf("SwitchBranch: %v", err)
	}
	if got, _ := CurrentBranch(repo); got != "feature" {
		t.Errorf("branch = %q, want feature", got)
	}
}

func TestSwitchBranchCreatesMissing(t *testing.T) {
	isolate(t)
	repo := initRepo(t)

	if err := SwitchBranch(repo, "feat/fresh"); err != nil {
		t.Fatalf("SwitchBranch: %v", err)
	}
	if got, _ := CurrentBranch(repo); got != "feat/fresh" {
		t.Errorf("branch = %q, want feat/fresh", got)
	}
}

func TestSwitchBranchTracksRemote(t *testing.T) {
	isolate(t)
	origin := initRepo(t)
	gitCmd(t, origin, "branch", "feature")

	clone := filepath.Join(t.TempDir(), "clone")
	if err := Clone(origin, clone); err != nil {
		t.Fatal(err)
	}
	if err := SwitchBranch(clone, "feature"); err != nil {
		t.Fatalf("SwitchBranch: %v", err)
	}
	if up := gitCmd(t, clone, "rev-parse", "--abbrev-ref", "feature@{upstream}"); up != "origin/feature" {
		t.Errorf("upstream = %q, want origin/feature", up)
	}
}

func TestCloneFailureIsReported(t *testing.T) {
	isolate(t)
	err := Clone(filepath.Join(t.TempDir(), "not-a-repo"), filepath.Join(t.TempDir(), "dest"))
	if err == nil {
		t.Fatal("Clone succeeded for a source that is not a repo")
	}
	if !strings.Contains(err.Error(), "git clone") {
		t.Errorf("error = %v, want it to name the failing command", err)
	}
}

func TestWorktreeRemove(t *testing.T) {
	isolate(t)
	repo := initRepo(t)
	dest := filepath.Join(t.TempDir(), "wt")
	if err := WorktreeAdd(repo, dest, "feature"); err != nil {
		t.Fatal(err)
	}

	if err := WorktreeRemove(repo, dest, false); err != nil {
		t.Fatalf("WorktreeRemove: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("worktree still on disk: %v", err)
	}
}

func TestWorktreeRemoveNeedsForceWithLocalChanges(t *testing.T) {
	isolate(t)
	repo := initRepo(t)
	dest := filepath.Join(t.TempDir(), "wt")
	if err := WorktreeAdd(repo, dest, "feature"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "file.txt"), []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := WorktreeRemove(repo, dest, false); err == nil {
		t.Fatal("WorktreeRemove succeeded despite uncommitted changes")
	}
	if err := WorktreeRemove(repo, dest, true); err != nil {
		t.Fatalf("forced WorktreeRemove: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("worktree still on disk after forced removal")
	}
}

func TestWorktreePruneClearsStaleAdminFiles(t *testing.T) {
	isolate(t)
	repo := initRepo(t)
	dest := filepath.Join(t.TempDir(), "wt")
	if err := WorktreeAdd(repo, dest, "feature"); err != nil {
		t.Fatal(err)
	}
	// Deleting the directory behind git's back leaves the administrative entry.
	if err := os.RemoveAll(dest); err != nil {
		t.Fatal(err)
	}
	if list := gitCmd(t, repo, "worktree", "list"); !strings.Contains(list, "prunable") {
		t.Skipf("this git does not report prunable worktrees:\n%s", list)
	}

	if err := WorktreePrune(repo); err != nil {
		t.Fatalf("WorktreePrune: %v", err)
	}
	if list := gitCmd(t, repo, "worktree", "list"); strings.Contains(list, dest) {
		t.Errorf("stale worktree still listed:\n%s", list)
	}
}

func TestRunReportsStderr(t *testing.T) {
	isolate(t)
	err := run(t.TempDir(), "rev-parse", "--verify", "HEAD")
	if err == nil {
		t.Fatal("run succeeded outside a repo")
	}
	if !strings.Contains(err.Error(), "rev-parse") {
		t.Errorf("error = %v, want it to name the failing command", err)
	}
}
