package main

import (
	"context"
	"fmt"
	"os"

	"github.com/endrilickollari/debtdrone-cli/internal/tui"
	"github.com/endrilickollari/debtdrone-cli/internal/update"
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
	// ── Root command ──────────────────────────────────────────────────────
	//
	// Cobra routing: when the user runs 'debtdrone' with no subcommand,
	// cobra finds no matching child and falls back to rootCmd.RunE. We
	// exploit this to make the interactive TUI the natural default while
	// still exposing 'debtdrone scan' as a first-class headless path.
	//
	// If the user runs 'debtdrone scan [path]', cobra matches that subcommand
	// and never calls rootCmd.RunE at all — so the TUI is never started.
	rootCmd := &cobra.Command{
		Use:   "debtdrone",
		Short: "DebtDrone — Technical Debt Analyzer",
		Long: `DebtDrone is a technical debt analyzer for your codebase.

Run without arguments to open the interactive TUI, where you can scan
repositories, browse results, manage configuration, and view scan history.

For CI/CD pipelines and scripted workflows, use the 'scan' subcommand:

  debtdrone scan ./myproject --format json`,

		// SilenceUsage prevents cobra from dumping the full usage block
		// alongside every RunE error — the error message is enough.
		SilenceUsage: true,

		// RunE is the TUI entry point. It is only reached when no subcommand
		// is provided (pure 'debtdrone' invocation).
		RunE: func(cmd *cobra.Command, args []string) error {
			// Offer an update prompt before entering the TUI so the user
			// is never surprised by a stale binary during interactive use.
			runAutoUpdate()

			fmt.Println("Starting DebtDrone TUI...")
			return tui.RunTUI()
		},
	}

	// Expose 'debtdrone --version' / '-v'. Cobra handles printing and
	// exiting automatically when this flag is present.
	rootCmd.Version = fmt.Sprintf("%s (commit %s, built at %s)", version, commit, date)

	// ── Subcommands ───────────────────────────────────────────────────────
	rootCmd.AddCommand(newScanCmd(), newInitCmd(), newConfigCmd(), newHistoryCmd())

	// Execute parses os.Args, routes to the matching command, and prints any
	// error to stderr. We only need to set the exit code here.
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
