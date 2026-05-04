package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/catdevman/gitgrove/internal/config"
	"github.com/catdevman/gitgrove/internal/grove"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor [grove]",
	Short: "Validate grove configuration",
	Long: `Doctor checks grove configuration for problems before you run sync.

If a grove name is provided, only that grove is checked. Otherwise all groves
are checked. Exits non-zero if any errors are found.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cacheDir, err := cfg.EffectiveCacheDir()
		if err != nil {
			return err
		}

		groves := cfg.Groves
		if len(args) == 1 {
			g, ok := groves[args[0]]
			if !ok {
				return fmt.Errorf("grove %q not found", args[0])
			}
			groves = map[string]*config.Grove{args[0]: g}
		}

		if len(groves) == 0 {
			fmt.Println("no groves configured")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "GROVE\tREPO\tCHECK\tSTATUS\tDETAIL")

		hasError := false
		labels := []string{"OK", "WARN", "ERROR"}
		for name, g := range groves {
			for _, issue := range grove.Doctor(name, g, cacheDir) {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					name, issue.Repo, issue.Check, labels[issue.Severity], issue.Message)
				if issue.Severity == grove.SeverityError {
					hasError = true
				}
			}
		}
		w.Flush()

		if hasError {
			return fmt.Errorf("one or more errors found")
		}
		return nil
	},
}
