package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/catdevman/gitgrove/internal/grove"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [grove]",
	Short: "Show sync status of groves",
	Long: `Status shows whether each repo's worktree is present on disk and whether
its checked-out branch matches the branch specified in the config.

Repos listed as MISSING have been added to the config but not yet synced.
Run 'gitgrove sync <grove>' to create their worktrees.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(cfg.Groves) == 0 {
			fmt.Println("no groves configured")
			return nil
		}

		var names []string
		if len(args) == 1 {
			name := args[0]
			if _, exists := cfg.Groves[name]; !exists {
				return fmt.Errorf("grove %q not found", name)
			}
			names = append(names, name)
		} else {
			names = groveNames()
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "GROVE\tREPO\tSTATUS\tCONFIG BRANCH\tACTUAL BRANCH")

		// Track which groves have unsynced repos so we can print hints after the table.
		needsSync := map[string]int{}

		for _, name := range names {
			g := cfg.Groves[name]
			s := grove.GetStatus(name, g)
			if len(s.Repos) == 0 {
				fmt.Fprintf(w, "%s\t(no repos)\t\t\t\n", name)
				continue
			}
			for _, rs := range s.Repos {
				label := statusLabel(rs)
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					name, rs.Name, label, rs.ConfigBranch, rs.ActualBranch)
				if !rs.Present {
					needsSync[name]++
				}
			}
		}
		w.Flush()

		if len(needsSync) > 0 {
			fmt.Println()
			for _, name := range names {
				n := needsSync[name]
				if n == 0 {
					continue
				}
				noun := "repo"
				if n > 1 {
					noun = "repos"
				}
				fmt.Printf("%d %s in %q not synced — run: gitgrove sync %s\n", n, noun, name, name)
			}
		}

		return nil
	},
}

func statusLabel(rs grove.RepoStatus) string {
	if !rs.Present {
		return "MISSING"
	}
	if !rs.BranchMatches {
		return "BRANCH MISMATCH"
	}
	return "OK"
}
