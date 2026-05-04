package grove

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/catdevman/gitgrove/internal/config"
	"github.com/catdevman/gitgrove/internal/git"
)

// Severity classifies a Doctor check result.
type Severity int

const (
	SeverityOK Severity = iota
	SeverityWarn
	SeverityError
)

// Issue is a single finding from Doctor.
type Issue struct {
	Repo     string
	Check    string
	Severity Severity
	Message  string
}

// Doctor validates the configuration for a grove and returns a list of findings.
// It is read-only and safe to call at any time.
func Doctor(name string, g *config.Grove, cacheDir string) []Issue {
	var issues []Issue
	seen := map[string]bool{}

	for _, repo := range g.Repos {
		dest := filepath.Join(g.Path, repo.Name)

		if config.IsRemoteSource(repo.Source) {
			clonePath, err := config.RemoteClonePath(cacheDir, repo.Source)
			if err != nil {
				issues = append(issues, Issue{repo.Name, "source", SeverityError, fmt.Sprintf("could not resolve clone path: %v", err)})
				continue
			}
			if git.IsGitRepo(clonePath) {
				issues = append(issues, Issue{repo.Name, "source", SeverityOK, fmt.Sprintf("cached at %s", clonePath)})
			} else {
				issues = append(issues, Issue{repo.Name, "source", SeverityOK, fmt.Sprintf("remote URL (will clone to %s on sync)", clonePath)})
			}
		} else if !git.IsGitRepo(repo.Source) {
			issues = append(issues, Issue{repo.Name, "source", SeverityError, fmt.Sprintf("not a git repo: %s", repo.Source)})
		} else {
			issues = append(issues, Issue{repo.Name, "source", SeverityOK, repo.Source})
		}

		if !config.IsRemoteSource(repo.Source) {
			if !git.BranchExists(repo.Source, repo.Branch) {
				issues = append(issues, Issue{repo.Name, "branch", SeverityWarn, fmt.Sprintf("branch %q not found — will be created on sync", repo.Branch)})
			} else {
				issues = append(issues, Issue{repo.Name, "branch", SeverityOK, repo.Branch})
			}
		}

		if seen[dest] {
			issues = append(issues, Issue{repo.Name, "dest", SeverityError, fmt.Sprintf("duplicate worktree path: %s", dest)})
		}
		seen[dest] = true
	}

	if err := os.MkdirAll(g.Path, 0o755); err != nil {
		issues = append(issues, Issue{"", "grove-path", SeverityError, fmt.Sprintf("cannot create %s: %v", g.Path, err)})
	}

	return issues
}

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
func Sync(name string, g *config.Grove, cacheDir string) error {
	if err := os.MkdirAll(g.Path, 0o755); err != nil {
		return fmt.Errorf("grove %s: could not create directory %s: %w", name, g.Path, err)
	}
	for _, repo := range g.Repos {
		effectiveSource := repo.Source
		if config.IsRemoteSource(repo.Source) {
			clonePath, err := config.RemoteClonePath(cacheDir, repo.Source)
			if err != nil {
				return fmt.Errorf("grove %s, repo %s: %w", name, repo.Name, err)
			}
			if !git.IsGitRepo(clonePath) {
				if err := os.MkdirAll(filepath.Dir(clonePath), 0o755); err != nil {
					return fmt.Errorf("grove %s, repo %s: could not create cache directory: %w", name, repo.Name, err)
				}
				fmt.Printf("  cloning %s → %s\n", repo.Source, clonePath)
				if err := git.Clone(repo.Source, clonePath); err != nil {
					return fmt.Errorf("grove %s, repo %s: %w", name, repo.Name, err)
				}
			}
			effectiveSource = clonePath
		}

		dest := filepath.Join(g.Path, repo.Name)
		if _, err := os.Stat(dest); err == nil {
			current, err := git.CurrentBranch(dest)
			if err != nil || current == repo.Branch {
				continue
			}
			fmt.Printf("  fixing branch drift %s: %s → %s\n", repo.Name, current, repo.Branch)
			if err := git.SwitchBranch(dest, repo.Branch); err != nil {
				return fmt.Errorf("grove %s, repo %s: %w", name, repo.Name, err)
			}
			continue
		}
		fmt.Printf("  adding worktree %s → %s (%s)\n", repo.Name, dest, repo.Branch)
		if err := git.WorktreeAdd(effectiveSource, dest, repo.Branch); err != nil {
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
