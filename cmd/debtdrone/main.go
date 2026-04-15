package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/endrilickollari/debtdrone-cli/internal/service"
	"github.com/endrilickollari/debtdrone-cli/internal/tui"
	"github.com/endrilickollari/debtdrone-cli/internal/update"
	"github.com/schollz/progressbar/v3"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func checkDependencies() {
	if _, err := exec.LookPath("trivy"); err != nil {
		fmt.Fprintln(os.Stderr, "⚠️  Trivy not found. Security scanning will be skipped.")
	}
}

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
	versionFlag := flag.Bool("version", false, "Print the version and exit")
	targetDir := flag.String("path", ".", "Path to the repository to analyze")
	failOn := flag.String("fail-on", "high", "Fail exit code if issues found with severity >= (low, medium, high, critical, none)")
	outputFormat := flag.String("output", "text", "Output format (text, json)")
	tuiMode := flag.Bool("tui", false, "Force TUI mode")
	flag.Parse()

	if *versionFlag {
		fmt.Fprintf(os.Stderr, "debtdrone version %s, commit %s, built at %s\n", version, commit, date)
		os.Exit(0)
	}

	hasCLIArgs := len(os.Args) > 1 && (len(flag.Args()) > 0 || os.Args[1] == "-tui")

	if !hasCLIArgs || *tuiMode {
		fmt.Println("Starting DebtDrone TUI...")
		runAutoUpdate()

		if err := tui.RunTUI(); err != nil {
			fmt.Fprintf(os.Stderr, "❌ TUI error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	checkDependencies()

	if len(flag.Args()) > 0 {
		*targetDir = flag.Args()[0]
	}

	if *outputFormat == "json" {
		log.SetOutput(ioutil.Discard)
	}

	absPath, err := filepath.Abs(*targetDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to resolve path: %v\n", err)
		os.Exit(1)
	}

	if *outputFormat == "text" {
		printBanner()
		fmt.Fprintf(os.Stderr, "🔍 Scanning repository at: %s\n", absPath)
	}

	svc := service.NewScanService()
	ctx := context.Background()
	ctx = context.WithValue(ctx, "isCLI", true)

	opts := service.ScanOptions{
		MaxComplexity: 15,
		SecurityScan:  true,
	}

	var bar *progressbar.ProgressBar
	allIssues, err := svc.Run(ctx, absPath, opts, func(p service.ScanProgress) {
		if *outputFormat == "text" {
			if bar == nil {
				bar = startSpinner(p.Total, "Analysing repository structure...")
			}
			bar.Add(1)
		}
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to analyze repository: %v\n", err)
		os.Exit(1)
	}

	if bar != nil {
		bar.Finish()
		fmt.Fprintln(os.Stderr)
	}

	printReport(allIssues, *outputFormat)

	if shouldFail(allIssues, *failOn) {
		fmt.Fprintln(os.Stderr, "\n❌ Quality Gate failed: Technical debt threshold exceeded.")
		os.Exit(1)
	}

	if *outputFormat == "text" {
		if len(allIssues) > 0 {
			fmt.Fprintf(os.Stderr, "\n⚠️  Scan completed with %d issues.\n", len(allIssues))
		} else {
			fmt.Fprintln(os.Stderr, "\n✅ Scan passed. No issues found.")
		}
	}
}
