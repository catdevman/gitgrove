package cmd

import (
	"fmt"

	"github.com/catdevman/gitgrove/internal/grove"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync [grove]",
	Short: "Create missing worktrees to match the config",
	Long: `Sync reconciles the filesystem with the config by creating any worktrees
that are defined in the config but not yet present on disk.

If a grove name is provided, only that grove is synced. Otherwise all groves
are synced.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cacheDir, err := cfg.EffectiveCacheDir()
		if err != nil {
			return err
		}

		if len(args) == 1 {
			name := args[0]
			g, exists := cfg.Groves[name]
			if !exists {
				return fmt.Errorf("grove %q not found", name)
			}
			fmt.Printf("syncing grove %q\n", name)
			return grove.Sync(name, g, cacheDir)
		}

		if len(cfg.Groves) == 0 {
			fmt.Println("no groves configured")
			return nil
		}
		for name, g := range cfg.Groves {
			fmt.Printf("syncing grove %q\n", name)
			if err := grove.Sync(name, g, cacheDir); err != nil {
				return err
			}
		}
		return nil
	},
}
