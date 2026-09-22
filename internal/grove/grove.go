package grove

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/catdevman/gitgrove/internal/config"
	"github.com/catdevman/gitgrove/internal/dirs"
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
			issues = append(issues, Issue{repo.Name, "source", SeverityError, fmt.Sprintf("not a git repo: %s — add it without a branch to copy it in as a plain directory", repo.Source)})
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

	for _, d := range g.Dirs {
		issues = append(issues, doctorDir(d)...)

		dest := filepath.Join(g.Path, d.Name)
		if seen[dest] {
			issues = append(issues, Issue{d.Name, "dest", SeverityError, fmt.Sprintf("duplicate path: %s", dest)})
		}
		seen[dest] = true
	}

	if err := checkPathCreatable(g.Path); err != nil {
		issues = append(issues, Issue{"", "grove-path", SeverityError, err.Error()})
	}

	return issues
}

// doctorDir validates a single plain directory entry.
func doctorDir(d config.Dir) []Issue {
	mode := d.EffectiveMode()
	if !dirs.ValidMode(mode) {
		return []Issue{{d.Name, "dir-mode", SeverityError, fmt.Sprintf("unknown mode %q (want %q or %q)", mode, dirs.ModeCopy, dirs.ModeSymlink)}}
	}

	info, err := os.Stat(d.Source)
	switch {
	case os.IsNotExist(err):
		return []Issue{{d.Name, "dir-source", SeverityError, fmt.Sprintf("does not exist: %s", d.Source)}}
	case err != nil:
		return []Issue{{d.Name, "dir-source", SeverityError, fmt.Sprintf("cannot stat %s: %v", d.Source, err)}}
	case !info.IsDir():
		return []Issue{{d.Name, "dir-source", SeverityError, fmt.Sprintf("not a directory: %s", d.Source)}}
	}

	issues := []Issue{{d.Name, "dir-source", SeverityOK, fmt.Sprintf("%s (%s)", d.Source, mode)}}
	if git.IsGitRepo(d.Source) {
		issues = append(issues, Issue{d.Name, "dir-source", SeverityWarn,
			"is a git repo — adding it as source:branch would give you a worktree instead of a copy"})
	}
	return issues
}

// checkPathCreatable reports whether path either already exists as a writable
// directory, or could be created. It does not touch the filesystem: doctor is
// read-only, so it walks up to the nearest existing ancestor and inspects that.
func checkPathCreatable(path string) error {
	p := filepath.Clean(path)
	for {
		info, err := os.Stat(p)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%s exists but is not a directory", p)
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("cannot stat %s: %v", p, err)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return fmt.Errorf("cannot create %s: no existing ancestor directory", path)
		}
		p = parent
	}
}

// RepoStatus describes whether a repo's worktree is in sync with the config.
type RepoStatus struct {
	Name          string
	ConfigBranch  string
	ActualBranch  string // empty if worktree is missing
	Present       bool
	BranchMatches bool
}

// DirState classifies what is on disk for a plain directory entry.
type DirState int

const (
	DirMissing  DirState = iota // nothing at the destination yet
	DirOK                       // present, and matching the configured mode
	DirDrifted                  // present, but not the way the config says
	DirConflict                 // something else is occupying the destination
)

// DirStatus describes whether a plain directory matches the config.
type DirStatus struct {
	Name   string
	Source string
	Mode   string
	State  DirState
	Actual string // what is actually on disk, when drifted
}

// Status describes the sync state of a single grove.
type Status struct {
	GroveName string
	GrovePath string
	Repos     []RepoStatus
	Dirs      []DirStatus
}

// Sync creates any worktrees defined in the grove config that do not yet exist
// on disk. It does not remove worktrees that are on disk but not in config.
func Sync(name string, g *config.Grove, cacheDir string) error {
	if err := os.MkdirAll(g.Path, 0o755); err != nil {
		return fmt.Errorf("grove %s: could not create directory %s: %w", name, g.Path, err)
	}
	for _, repo := range g.Repos {
		effectiveSource, err := resolveSource(repo, cacheDir)
		if err != nil {
			return fmt.Errorf("grove %s, repo %s: %w", name, repo.Name, err)
		}
		if !git.IsGitRepo(effectiveSource) && config.IsRemoteSource(repo.Source) {
			if err := os.MkdirAll(filepath.Dir(effectiveSource), 0o755); err != nil {
				return fmt.Errorf("grove %s, repo %s: could not create cache directory: %w", name, repo.Name, err)
			}
			fmt.Printf("  cloning %s → %s\n", repo.Source, effectiveSource)
			if err := git.Clone(repo.Source, effectiveSource); err != nil {
				return fmt.Errorf("grove %s, repo %s: %w", name, repo.Name, err)
			}
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

	for _, d := range g.Dirs {
		action, err := dirs.Ensure(d.Source, filepath.Join(g.Path, d.Name), d.EffectiveMode())
		if err != nil {
			return fmt.Errorf("grove %s, dir %s: %w", name, d.Name, err)
		}
		if action != "" {
			fmt.Printf("  %s\n", action)
		}
	}
	return nil
}

// resolveSource returns the local repository path backing a repo entry. For a
// remote source that is the path it is (or will be) cloned to under cacheDir;
// for a local source it is the source itself.
func resolveSource(repo config.Repo, cacheDir string) (string, error) {
	if !config.IsRemoteSource(repo.Source) {
		return repo.Source, nil
	}
	return config.RemoteClonePath(cacheDir, repo.Source)
}

// Remove deletes all worktrees for the grove. If force is true, uncommitted
// changes are discarded.
func Remove(name string, g *config.Grove, cacheDir string, force bool) error {
	for _, repo := range g.Repos {
		dest := filepath.Join(g.Path, repo.Name)
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			continue
		}
		// Worktree operations must run against the local repo that owns the
		// worktree — for a remote source that is the cached clone, never the
		// URL in the config.
		source, err := resolveSource(repo, cacheDir)
		if err != nil {
			return fmt.Errorf("grove %s, repo %s: %w", name, repo.Name, err)
		}
		if !git.IsGitRepo(source) {
			fmt.Printf("  skipping %s: source repo %s is missing\n", repo.Name, source)
			continue
		}
		fmt.Printf("  removing worktree %s\n", dest)
		if err := git.WorktreeRemove(source, dest, force); err != nil {
			return fmt.Errorf("grove %s, repo %s: %w", name, repo.Name, err)
		}
		_ = git.WorktreePrune(source)
	}

	for _, d := range g.Dirs {
		dest := filepath.Join(g.Path, d.Name)
		if _, err := os.Lstat(dest); os.IsNotExist(err) {
			continue
		}
		fmt.Printf("  removing %s\n", dest)
		if err := dirs.Remove(dest, d.EffectiveMode(), force); err != nil {
			return fmt.Errorf("grove %s, dir %s: %w", name, d.Name, err)
		}
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
	for _, d := range g.Dirs {
		s.Dirs = append(s.Dirs, dirStatus(g.Path, d))
	}
	return s
}

func dirStatus(grovePath string, d config.Dir) DirStatus {
	ds := DirStatus{Name: d.Name, Source: d.Source, Mode: d.EffectiveMode()}
	st, err := dirs.Inspect(filepath.Join(grovePath, d.Name))
	switch {
	case err != nil || !st.Present:
		ds.State = DirMissing
	case ds.Mode == dirs.ModeCopy:
		// The copy is grove-owned, so a real directory there is exactly what
		// sync made. A symlink is left over from symlink mode.
		if st.IsSymlink {
			ds.State = DirDrifted
			ds.Actual = "symlink to " + st.Target
		} else {
			ds.State = DirOK
		}
	case !st.IsSymlink:
		ds.State = DirConflict
	case st.Target == filepath.Clean(d.Source):
		ds.State = DirOK
	default:
		ds.State = DirDrifted
		ds.Actual = st.Target
	}
	return ds
}
