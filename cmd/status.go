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
Run 'gitgrove sync <grove>' to create their worktrees.

Plain directories are listed in a second table when a grove has any.`,
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

		// Track which groves have unsynced entries so we can print hints after
		// the tables.
		needsSync := map[string]int{}
		statuses := make([]grove.Status, 0, len(names))

		for _, name := range names {
			s := grove.GetStatus(name, cfg.Groves[name])
			statuses = append(statuses, s)
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

		printDirs(statuses, needsSync)

		if len(needsSync) > 0 {
			fmt.Println()
			for _, name := range names {
				n := needsSync[name]
				if n == 0 {
					continue
				}
				noun := "entry"
				if n > 1 {
					noun = "entries"
				}
				fmt.Printf("%d %s in %q not synced — run: gitgrove sync %s\n", n, noun, name, name)
			}
		}

		return nil
	},
}

// printDirs writes the plain-directory table, but only when at least one grove
// has directories — groves without them keep the original single-table output.
func printDirs(statuses []grove.Status, needsSync map[string]int) {
	any := false
	for _, s := range statuses {
		if len(s.Dirs) > 0 {
			any = true
			break
		}
	}
	if !any {
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Println()
	fmt.Fprintln(w, "GROVE\tDIR\tSTATUS\tMODE\tSOURCE")
	for _, s := range statuses {
		for _, ds := range s.Dirs {
			source := ds.Source
			if ds.Actual != "" {
				source = fmt.Sprintf("%s (found %s)", ds.Source, ds.Actual)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				s.GroveName, ds.Name, dirStateLabel(ds.State), ds.Mode, source)
			if ds.State == grove.DirMissing {
				needsSync[s.GroveName]++
			}
		}
	}
	w.Flush()
}

func dirStateLabel(state grove.DirState) string {
	switch state {
	case grove.DirOK:
		return "OK"
	case grove.DirDrifted:
		return "DRIFTED"
	case grove.DirConflict:
		return "CONFLICT"
	default:
		return "MISSING"
	}
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
