package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/catdevman/gitgrove/internal/config"
	"github.com/catdevman/gitgrove/internal/grove"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new grove",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if _, exists := cfg.Groves[name]; exists {
			return fmt.Errorf("grove %q already exists", name)
		}
		grovePath, _ := cmd.Flags().GetString("path")
		if grovePath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			grovePath = filepath.Join(home, "groves", name)
		}
		cfg.Groves[name] = &config.Grove{Path: grovePath}
		if err := saveConfig(); err != nil {
			return err
		}
		fmt.Printf("created grove %q at %s\n", name, grovePath)
		return nil
	},
}

var deleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a grove (use --prune to also remove worktrees from disk)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		g, exists := cfg.Groves[name]
		if !exists {
			return fmt.Errorf("grove %q not found", name)
		}
		prune, _ := cmd.Flags().GetBool("prune")
		if prune {
			force, _ := cmd.Flags().GetBool("force")
			cacheDir, err := cfg.EffectiveCacheDir()
			if err != nil {
				return err
			}
			fmt.Printf("pruning worktrees for grove %q\n", name)
			if err := grove.Remove(name, g, cacheDir, force); err != nil {
				return err
			}
		}
		delete(cfg.Groves, name)
		if err := saveConfig(); err != nil {
			return err
		}
		fmt.Printf("deleted grove %q\n", name)
		return nil
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all groves",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(cfg.Groves) == 0 {
			fmt.Println("no groves configured")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "GROVE\tPATH\tREPOS")
		for _, name := range groveNames() {
			g := cfg.Groves[name]
			fmt.Fprintf(w, "%s\t%s\t%d\n", name, g.Path, len(g.Repos))
		}
		return w.Flush()
	},
}

func init() {
	createCmd.Flags().String("path", "", "path for the grove directory (default: ~/groves/<name>)")
	deleteCmd.Flags().Bool("prune", false, "remove worktrees from disk")
	deleteCmd.Flags().Bool("force", false, "force removal even with uncommitted changes (requires --prune)")
}
