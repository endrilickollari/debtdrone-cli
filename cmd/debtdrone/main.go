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

	"github.com/endrilickollari/debtdrone-cli/internal/analysis"
	"github.com/endrilickollari/debtdrone-cli/internal/analysis/analyzers"
	"github.com/endrilickollari/debtdrone-cli/internal/analysis/analyzers/security"
	"github.com/endrilickollari/debtdrone-cli/internal/git"
	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/endrilickollari/debtdrone-cli/internal/store/memory"
	"github.com/google/uuid"
	"github.com/schollz/progressbar/v3"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func checkDependencies() {
	if _, err := exec.LookPath("git"); err != nil {
		fmt.Fprintln(os.Stderr, "❌ Error: git is required but not installed.")
		os.Exit(1)
	}

	if _, err := exec.LookPath("trivy"); err != nil {
		fmt.Fprintln(os.Stderr, "⚠️  Trivy not found. Security scanning will be skipped.")
	}
}

func main() {

	versionFlag := flag.Bool("version", false, "Print the version and exit")
	targetDir := flag.String("path", ".", "Path to the repository to analyze")
	failOn := flag.String("fail-on", "high", "Fail exit code if issues found with severity >= (low, medium, high, critical, none)")
	outputFormat := flag.String("output", "text", "Output format (text, json)")
	flag.Parse()

	if *versionFlag {
		fmt.Fprintf(os.Stderr, "debtdrone version %s, commit %s, built at %s\n", version, commit, date)
		os.Exit(0)
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

	complexityStore := memory.NewInMemoryComplexityStore()

	lineCounter := analyzers.NewLineCounter()
	complexityAnalyzer := analyzers.NewComplexityAnalyzer(complexityStore)
	trivyAnalyzer := security.NewTrivyAnalyzer()

	gitService := git.NewService()
	repo, err := gitService.OpenLocal(absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to open repository: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, "analysisRunID", uuid.New())
	ctx = context.WithValue(ctx, "repositoryID", uuid.New())
	ctx = context.WithValue(ctx, "userID", uuid.New())
	ctx = context.WithValue(ctx, "isCLI", true)

	var allIssues []models.TechnicalDebtIssue

	analyzersList := []analysis.Analyzer{lineCounter, complexityAnalyzer, trivyAnalyzer}

	var bar *progressbar.ProgressBar
	if *outputFormat == "text" {
		bar = startSpinner(len(analyzersList), "Analysing repository structure...")
	}

	for _, analyzer := range analyzersList {
		result, err := analyzer.Analyze(ctx, repo)
		if err != nil {
		} else {
			allIssues = append(allIssues, result.Issues...)
		}

		if bar != nil {
			bar.Add(1)
		}
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
