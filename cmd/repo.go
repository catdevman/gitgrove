package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/catdevman/gitgrove/internal/config"
	"github.com/catdevman/gitgrove/internal/grove"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add <grove> <source:branch> [<source:branch> ...]",
	Short: "Add one or more repos to a grove",
	Long: `Add repos to a grove. Each repo is specified as source:branch where source
is the path to the local repo and branch is the branch to check out.

Examples:
  gitgrove add my-feature ~/code/backend:feat/api
  gitgrove add my-feature ~/code/backend:feat/api ~/code/frontend:feat/ui`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		groveName := args[0]
		g, exists := cfg.Groves[groveName]
		if !exists {
			return fmt.Errorf("grove %q not found; create it first with: gitgrove create %s", groveName, groveName)
		}

		type entry struct {
			source, branch, name string
		}
		var entries []entry

		for _, arg := range args[1:] {
			idx := strings.LastIndex(arg, ":")
			if idx == -1 {
				return fmt.Errorf("invalid argument %q: expected source:branch", arg)
			}
			source := arg[:idx]
			branch := arg[idx+1:]
			if source == "" || branch == "" {
				return fmt.Errorf("invalid argument %q: source and branch must both be non-empty", arg)
			}
			name := filepath.Base(source)
			entries = append(entries, entry{source, branch, name})
		}

		// Check for duplicates against existing repos and within the new batch.
		seen := make(map[string]bool)
		for _, r := range g.Repos {
			seen[r.Name] = true
		}
		for _, e := range entries {
			if seen[e.name] {
				return fmt.Errorf("repo %q already exists in grove %q", e.name, groveName)
			}
			seen[e.name] = true
		}

		for _, e := range entries {
			g.Repos = append(g.Repos, config.Repo{
				Name:   e.name,
				Source: e.source,
				Branch: e.branch,
			})
			fmt.Printf("added repo %q (branch: %s) to grove %q\n", e.name, e.branch, groveName)
		}

		if err := saveConfig(); err != nil {
			return err
		}
		fmt.Printf("run `gitgrove sync %s` to create the worktree(s)\n", groveName)
		return nil
	},
}

var removeCmd = &cobra.Command{
	Use:   "remove <grove> <name>",
	Short: "Remove a repo from a grove",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		groveName := args[0]
		repoName := args[1]

		g, exists := cfg.Groves[groveName]
		if !exists {
			return fmt.Errorf("grove %q not found", groveName)
		}

		idx := -1
		for i, r := range g.Repos {
			if r.Name == repoName {
				idx = i
				break
			}
		}
		if idx == -1 {
			return fmt.Errorf("repo %q not found in grove %q", repoName, groveName)
		}

		prune, _ := cmd.Flags().GetBool("prune")
		if prune {
			force, _ := cmd.Flags().GetBool("force")
			cacheDir, err := cfg.EffectiveCacheDir()
			if err != nil {
				return err
			}
			singleGrove := &config.Grove{
				Path:  g.Path,
				Repos: []config.Repo{g.Repos[idx]},
			}
			if err := grove.Remove(groveName, singleGrove, cacheDir, force); err != nil {
				return err
			}
		}

		g.Repos = append(g.Repos[:idx], g.Repos[idx+1:]...)
		if err := saveConfig(); err != nil {
			return err
		}
		fmt.Printf("removed repo %q from grove %q\n", repoName, groveName)
		return nil
	},
}

func init() {
	removeCmd.Flags().Bool("prune", false, "remove the worktree from disk")
	removeCmd.Flags().Bool("force", false, "force removal even with uncommitted changes (requires --prune)")
}
