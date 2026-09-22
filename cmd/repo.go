package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/catdevman/gitgrove/internal/config"
	"github.com/catdevman/gitgrove/internal/dirs"
	"github.com/catdevman/gitgrove/internal/git"
	"github.com/catdevman/gitgrove/internal/grove"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add <grove> <source:branch|dir> [...]",
	Short: "Add one or more repos or plain directories to a grove",
	Long: `Add repos and plain directories to a grove.

A repo is specified as source:branch, where source is a local path or a remote
URL and branch is the branch to check out. It is linked in as a git worktree.

An argument with no branch is a plain directory — reference docs, a code
example, anything with no repository behind it. It is copied into the grove:
the grove is disposable, so the copy is pruned along with it and nothing done
inside the grove reaches the original. Pass --symlink to point at the live
directory instead, so edits land in the source.

Examples:
  gitgrove add my-feature ~/code/backend:feat/api
  gitgrove add my-feature ~/code/backend:feat/api ~/code/frontend:feat/ui
  gitgrove add my-feature ~/notes/api-docs
  gitgrove add my-feature ~/notes/api-docs --name docs
  gitgrove add my-feature ~/code/some-example --symlink`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		groveName := args[0]
		g, exists := cfg.Groves[groveName]
		if !exists {
			return fmt.Errorf("grove %q not found; create it first with: gitgrove create %s", groveName, groveName)
		}

		sources := args[1:]
		name, _ := cmd.Flags().GetString("name")
		if name != "" && len(sources) > 1 {
			return fmt.Errorf("--name can only be used when adding a single repo or directory")
		}
		asDir, _ := cmd.Flags().GetBool("dir")
		symlink, _ := cmd.Flags().GetBool("symlink")

		mode := dirs.ModeCopy
		if symlink {
			mode = dirs.ModeSymlink
		}

		// Repos and directories share the grove directory, so a name has to be
		// unique across both the existing entries and the new batch.
		seen := make(map[string]bool)
		for _, r := range g.Repos {
			seen[r.Name] = true
		}
		for _, d := range g.Dirs {
			seen[d.Name] = true
		}

		var repos []config.Repo
		var plainDirs []config.Dir

		for _, arg := range sources {
			source, branch, err := parseAddArg(arg, asDir)
			if err != nil {
				return err
			}

			entryName := name
			if entryName == "" {
				entryName = filepath.Base(filepath.Clean(source))
			}
			if entryName == "" || entryName == "." || entryName == string(filepath.Separator) {
				return fmt.Errorf("could not derive a name from %q — pass --name", arg)
			}
			if seen[entryName] {
				return fmt.Errorf("%q already exists in grove %q", entryName, groveName)
			}
			seen[entryName] = true

			if branch == "" {
				if err := checkDirSource(source, asDir); err != nil {
					return err
				}
				plainDirs = append(plainDirs, config.Dir{Name: entryName, Source: source, Mode: mode})
				continue
			}
			repos = append(repos, config.Repo{Name: entryName, Source: source, Branch: branch})
		}

		if symlink && len(plainDirs) == 0 {
			return fmt.Errorf("--symlink only applies to plain directories; repos are always linked as worktrees")
		}

		g.Repos = append(g.Repos, repos...)
		g.Dirs = append(g.Dirs, plainDirs...)
		if err := saveConfig(); err != nil {
			return err
		}
		for _, r := range repos {
			fmt.Printf("added repo %q (branch: %s) to grove %q\n", r.Name, r.Branch, groveName)
		}
		for _, d := range plainDirs {
			fmt.Printf("added directory %q (%s) to grove %q\n", d.Name, d.Mode, groveName)
		}
		fmt.Printf("run `gitgrove sync %s` to materialize it\n", groveName)
		return nil
	},
}

// parseAddArg splits one add argument into a source and a branch. An empty
// branch means the argument is a plain directory rather than a repo.
//
// The branch separator is the last colon, but only the part of the argument
// after any scheme or host: "git@host:owner/repo" and "https://host/owner/repo"
// carry colons of their own that are not branch separators.
func parseAddArg(arg string, asDir bool) (source, branch string, err error) {
	if asDir {
		return arg, "", nil
	}
	if config.IsRemoteSource(arg) {
		start := strings.Index(arg, "://")
		if start != -1 {
			start += len("://")
		} else {
			// git@host:owner/repo — the first colon separates host from path.
			start = strings.Index(arg, ":") + 1
		}
		idx := strings.LastIndex(arg[start:], ":")
		if idx == -1 {
			return "", "", fmt.Errorf("invalid argument %q: a remote repo needs a branch, as %s:main", arg, arg)
		}
		idx += start
		source, branch = arg[:idx], arg[idx+1:]
		if branch == "" {
			return "", "", fmt.Errorf("invalid argument %q: branch must be non-empty", arg)
		}
		return source, branch, nil
	}

	idx := strings.LastIndex(arg, ":")
	if idx == -1 {
		// No branch: a plain directory.
		return arg, "", nil
	}
	source, branch = arg[:idx], arg[idx+1:]
	if source == "" || branch == "" {
		return "", "", fmt.Errorf("invalid argument %q: source and branch must both be non-empty", arg)
	}
	return source, branch, nil
}

// checkDirSource rejects a git repo added as a plain directory. Copying a repo
// wholesale — .git and all — is almost never what was meant when a branch was
// simply forgotten, so it takes an explicit --dir to go through with it.
func checkDirSource(source string, asDir bool) error {
	if asDir || !git.IsGitRepo(source) {
		return nil
	}
	return fmt.Errorf("%s is a git repo: add it as %s:<branch> for a worktree, or pass --dir to copy it in as a plain directory", source, source)
}

var removeCmd = &cobra.Command{
	Use:   "remove <grove> <name>",
	Short: "Remove a repo or directory from a grove",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		groveName := args[0]
		entryName := args[1]

		g, exists := cfg.Groves[groveName]
		if !exists {
			return fmt.Errorf("grove %q not found", groveName)
		}

		// A name is unique across repos and directories, so whichever list
		// holds it is the entry being removed.
		repoIdx, dirIdx := -1, -1
		for i, r := range g.Repos {
			if r.Name == entryName {
				repoIdx = i
				break
			}
		}
		if repoIdx == -1 {
			for i, d := range g.Dirs {
				if d.Name == entryName {
					dirIdx = i
					break
				}
			}
		}
		if repoIdx == -1 && dirIdx == -1 {
			return fmt.Errorf("%q not found in grove %q", entryName, groveName)
		}

		prune, _ := cmd.Flags().GetBool("prune")
		if prune {
			force, _ := cmd.Flags().GetBool("force")
			cacheDir, err := cfg.EffectiveCacheDir()
			if err != nil {
				return err
			}
			singleGrove := &config.Grove{Path: g.Path}
			if repoIdx != -1 {
				singleGrove.Repos = []config.Repo{g.Repos[repoIdx]}
			} else {
				singleGrove.Dirs = []config.Dir{g.Dirs[dirIdx]}
			}
			if err := grove.Remove(groveName, singleGrove, cacheDir, force); err != nil {
				return err
			}
		}

		kind := "repo"
		if repoIdx != -1 {
			g.Repos = append(g.Repos[:repoIdx], g.Repos[repoIdx+1:]...)
		} else {
			kind = "directory"
			g.Dirs = append(g.Dirs[:dirIdx], g.Dirs[dirIdx+1:]...)
		}
		if err := saveConfig(); err != nil {
			return err
		}
		fmt.Printf("removed %s %q from grove %q\n", kind, entryName, groveName)
		return nil
	},
}

func init() {
	addCmd.Flags().String("name", "", "name inside the grove (default: the source basename)")
	addCmd.Flags().Bool("symlink", false, "symlink a plain directory instead of copying it")
	addCmd.Flags().Bool("dir", false, "treat the source as a plain directory even if it is a git repo")
	removeCmd.Flags().Bool("prune", false, "remove the worktree or directory from disk")
	removeCmd.Flags().Bool("force", false, "force removal even with uncommitted changes (requires --prune)")
}
