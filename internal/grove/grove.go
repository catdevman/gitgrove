package grove

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/catdevman/gitgrove/internal/config"
	"github.com/catdevman/gitgrove/internal/git"
)

// RepoStatus describes whether a repo's worktree is in sync with the config.
type RepoStatus struct {
	Name          string
	ConfigBranch  string
	ActualBranch  string // empty if worktree is missing
	Present       bool
	BranchMatches bool
}

// Status describes the sync state of a single grove.
type Status struct {
	GroveName string
	GrovePath string
	Repos     []RepoStatus
}

// Sync creates any worktrees defined in the grove config that do not yet exist
// on disk. It does not remove worktrees that are on disk but not in config.
func Sync(name string, g *config.Grove) error {
	if err := os.MkdirAll(g.Path, 0o755); err != nil {
		return fmt.Errorf("grove %s: could not create directory %s: %w", name, g.Path, err)
	}
	for _, repo := range g.Repos {
		dest := filepath.Join(g.Path, repo.Name)
		if _, err := os.Stat(dest); err == nil {
			// Already exists — skip.
			continue
		}
		fmt.Printf("  adding worktree %s → %s (%s)\n", repo.Name, dest, repo.Branch)
		if err := git.WorktreeAdd(repo.Source, dest, repo.Branch); err != nil {
			return fmt.Errorf("grove %s, repo %s: %w", name, repo.Name, err)
		}
	}
	return nil
}

// Remove deletes all worktrees for the grove. If force is true, uncommitted
// changes are discarded.
func Remove(name string, g *config.Grove, force bool) error {
	for _, repo := range g.Repos {
		dest := filepath.Join(g.Path, repo.Name)
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			continue
		}
		fmt.Printf("  removing worktree %s\n", dest)
		if err := git.WorktreeRemove(repo.Source, dest, force); err != nil {
			return fmt.Errorf("grove %s, repo %s: %w", name, repo.Name, err)
		}
		_ = git.WorktreePrune(repo.Source)
	}
	return nil
}

// GetStatus returns the sync status of a grove.
func GetStatus(name string, g *config.Grove) Status {
	s := Status{GroveName: name, GrovePath: g.Path}
	for _, repo := range g.Repos {
		dest := filepath.Join(g.Path, repo.Name)
		rs := RepoStatus{
			Name:         repo.Name,
			ConfigBranch: repo.Branch,
		}
		if _, err := os.Stat(dest); err == nil {
			rs.Present = true
			branch, err := git.CurrentBranch(dest)
			if err == nil {
				rs.ActualBranch = branch
				rs.BranchMatches = branch == repo.Branch
			}
		}
		s.Repos = append(s.Repos, rs)
	}
	return s
}
