package main

import (
	"context"
	"fmt"
	"os"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/tui"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/update"
	"github.com/spf13/cobra"
)

// Build-time variables injected by the linker via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// runAutoUpdate checks GitHub for a newer release and offers an interactive
// upgrade prompt. It is called from the root command's RunE so it only fires
// when the user launches the TUI, never during a headless scan.
func runAutoUpdate() {
	ctx := context.Background()
	info, err := update.CheckForUpdate(ctx, version)
	if err != nil {
		return
	}
	if info.Available {
		fmt.Printf("🔔 New version available: %s\n", info.Version)
		fmt.Print("Would you like to update now? (y/n): ")

		var response string
		fmt.Scanln(&response)

		if response == "y" || response == "Y" {
			fmt.Println("🔄 Updating...")
			if err := update.PerformUpdate(ctx); err != nil {
				fmt.Printf("❌ Update failed: %v\n", err)
			} else {
				fmt.Println("✅ Update installed! Please restart the application.")
				os.Exit(0)
			}
		}
	}
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "debtdrone",
		Short: "DebtDrone — Technical Debt Analyzer",
		Long: `DebtDrone is a technical debt analyzer for your codebase.

Run without arguments to open the interactive TUI, where you can scan
repositories, browse results, manage configuration, and view scan history.

For CI/CD pipelines and scripted workflows, use the 'scan' subcommand:

  debtdrone scan ./myproject --format json`,

		SilenceUsage: true,

		RunE: func(cmd *cobra.Command, args []string) error {
			runAutoUpdate()

			fmt.Println("Starting DebtDrone TUI...")
			return tui.RunTUI()
		},
	}

	rootCmd.Version = fmt.Sprintf("%s (commit %s, built at %s)", version, commit, date)

	rootCmd.AddCommand(newScanCmd(), newInitCmd(), newConfigCmd(), newHistoryCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
