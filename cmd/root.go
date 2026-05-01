package cmd

import (
	"fmt"
	"os"

	"github.com/catdevman/gitgrove/internal/config"
	"github.com/spf13/cobra"
)

var cfgPath string
var cfg *config.Config

var rootCmd = &cobra.Command{
	Use:   "gitgrove",
	Short: "Manage git worktree groves across multiple repos",
	Long: `GitGrove creates task-scoped workspaces ("groves") containing linked git
worktrees from multiple repositories — without duplicating any data.

A grove is a directory you can give an AI coding tool access to, so it can
see all the repos relevant to a task in one place.`,
	SilenceUsage: true,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	defaultPath, _ := config.DefaultPath()
	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", defaultPath, "path to config file")
	cobra.OnInitialize(loadConfig)

	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(statusCmd)
}

func loadConfig() {
	var err error
	cfg, err = config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func saveConfig() error {
	return config.Save(cfg, cfgPath)
}
