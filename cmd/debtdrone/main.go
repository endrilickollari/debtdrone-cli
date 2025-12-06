package main

import (
	"context"
	"flag"
	"fmt"
	"os"
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

func main() {

	targetDir := flag.String("path", ".", "Path to the repository to analyze")
	failOn := flag.String("fail-on", "high", "Fail exit code if issues found with severity >= (low, medium, high, critical, none)")
	outputFormat := flag.String("output", "text", "Output format (text, json)")
	flag.Parse()

	absPath, err := filepath.Abs(*targetDir)
	if err != nil {
		fmt.Printf("❌ Failed to resolve path: %v\n", err)
		os.Exit(1)
	}

	if *outputFormat == "text" {
		printBanner()
		fmt.Printf("🔍 Scanning repository at: %s\n", absPath)
	}

	// runStore := memory.NewInMemoryRunStore()
	complexityStore := memory.NewInMemoryComplexityStore()

	lineCounter := analyzers.NewLineCounter()
	complexityAnalyzer := analyzers.NewComplexityAnalyzer(complexityStore)
	trivyAnalyzer := security.NewTrivyAnalyzer()

	gitService := git.NewService()
	repo, err := gitService.OpenLocal(absPath)
	if err != nil {
		fmt.Printf("❌ Failed to open repository: %v\n", err)
		os.Exit(1)
	}

	// Run Analysis Directly (Skip the Engine Worker Pool for CLI)
	ctx := context.Background()
	// Add required context values that analyzers expect
	ctx = context.WithValue(ctx, "analysisRunID", uuid.New())
	ctx = context.WithValue(ctx, "repositoryID", uuid.New())
	ctx = context.WithValue(ctx, "userID", uuid.New())

	var allIssues []models.TechnicalDebtIssue

	analyzersList := []analysis.Analyzer{lineCounter, complexityAnalyzer, trivyAnalyzer}

	var bar *progressbar.ProgressBar
	if *outputFormat == "text" {
		bar = startSpinner(len(analyzersList), "Analysing repository structure...")
	}

	for _, analyzer := range analyzersList {
		// fmt.Printf("   👉 Running %s...\n", analyzer.Name())
		result, err := analyzer.Analyze(ctx, repo)
		if err != nil {
			// Log error but continue with other analyzers
		} else {
			allIssues = append(allIssues, result.Issues...)
		}

		if bar != nil {
			bar.Add(1)
		}
	}

	if bar != nil {
		bar.Finish()
		fmt.Println()
	}

	printReport(allIssues, *outputFormat)

	if shouldFail(allIssues, *failOn) {
		fmt.Println("\n❌ Quality Gate failed: Technical debt threshold exceeded.")
		os.Exit(1)
	}

	if len(allIssues) > 0 {
		fmt.Printf("\n⚠️  Scan completed with %d issues.\n", len(allIssues))
	} else {
		fmt.Println("\n✅ Scan passed. No issues found.")
	}
}
