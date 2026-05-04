package git

import (
	"bytes"
	"fmt"
	"os/exec"
)

// WorktreeAdd creates a linked worktree at destPath for the given branch,
// using the repo at sourcePath as the git directory. If the branch does not
// yet exist it is created from HEAD via -b.
func WorktreeAdd(sourcePath, destPath, branch string) error {
	if BranchExists(sourcePath, branch) {
		return run(sourcePath, "worktree", "add", destPath, branch)
	}
	return run(sourcePath, "worktree", "add", "-b", branch, destPath)
}

// BranchExists reports whether branch is a valid ref in the repo at repoPath.
func BranchExists(repoPath, branch string) bool {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--verify", branch)
	return cmd.Run() == nil
}

// IsGitRepo reports whether path is a git repository.
func IsGitRepo(path string) bool {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

// SwitchBranch checks out branch in the worktree at worktreePath.
// If the branch does not exist locally it is created from HEAD via -b.
func SwitchBranch(worktreePath, branch string) error {
	if BranchExists(worktreePath, branch) {
		return run(worktreePath, "checkout", branch)
	}
	return run(worktreePath, "checkout", "-b", branch)
}

// Clone clones remoteURL into localPath.
func Clone(remoteURL, localPath string) error {
	cmd := exec.Command("git", "clone", remoteURL, localPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone %s: %w — %s", remoteURL, err, stderr.String())
	}
	return nil
}

// WorktreeRemove removes the worktree at destPath.
// The force flag removes the worktree even if it has uncommitted changes.
func WorktreeRemove(sourcePath, destPath string, force bool) error {
	args := []string{"worktree", "remove", destPath}
	if force {
		args = append(args, "--force")
	}
	return run(sourcePath, args...)
}

// WorktreePrune cleans up stale worktree administrative files.
func WorktreePrune(sourcePath string) error {
	return run(sourcePath, "worktree", "prune")
}

// CurrentBranch returns the name of the branch currently checked out in the
// worktree at the given path.
func CurrentBranch(worktreePath string) (string, error) {
	cmd := exec.Command("git", "-C", worktreePath, "rev-parse", "--abbrev-ref", "HEAD")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git rev-parse in %s: %w — %s", worktreePath, err, stderr.String())
	}
	branch := string(bytes.TrimSpace(stdout.Bytes()))
	return branch, nil
}

func run(repoPath string, args ...string) error {
	fullArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.Command("git", fullArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %v: %w — %s", args, err, stderr.String())
	}
	return nil
}
