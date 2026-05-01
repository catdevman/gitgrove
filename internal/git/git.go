package git

import (
	"bytes"
	"fmt"
	"os/exec"
)

// WorktreeAdd creates a linked worktree at destPath for the given branch,
// using the repo at sourcePath as the git directory.
func WorktreeAdd(sourcePath, destPath, branch string) error {
	return run(sourcePath, "worktree", "add", destPath, branch)
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
