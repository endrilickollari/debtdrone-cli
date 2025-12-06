package main

import (
	"encoding/json"
	"fmt"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/fatih/color"
)

func printReport(issues []models.TechnicalDebtIssue, format string) {
	if format == "json" {
		output, _ := json.MarshalIndent(issues, "", "  ")
		fmt.Println(string(output))
		return
	}

	fmt.Printf("\n📊 Analysis Report\n")
	fmt.Printf("==================\n")
	fmt.Printf("Total Issues: %d\n\n", len(issues))

	for _, issue := range issues {
		var severityColor func(a ...interface{}) string
		switch issue.Severity {
		case "critical":
			severityColor = color.New(color.FgRed, color.Bold).SprintFunc()
		case "high":
			severityColor = color.New(color.FgRed).SprintFunc()
		case "medium":
			severityColor = color.New(color.FgYellow).SprintFunc()
		case "low":
			severityColor = color.New(color.FgBlue).SprintFunc()
		default:
			severityColor = color.New(color.FgWhite).SprintFunc()
		}

		fmt.Printf("[%s] %s: %s\n", severityColor(issue.Severity), issue.FilePath, issue.Message)
	}
}

func shouldFail(issues []models.TechnicalDebtIssue, threshold string) bool {
	if threshold == "none" {
		return false
	}

	severityMap := map[string]int{
		"info":     0,
		"low":      1,
		"medium":   2,
		"high":     3,
		"critical": 4,
	}

	thresholdVal, ok := severityMap[threshold]
	if !ok {
		thresholdVal = 3
	}

	for _, issue := range issues {
		if val, ok := severityMap[issue.Severity]; ok {
			if val >= thresholdVal {
				return true
			}
		}
	}
	return false
}
