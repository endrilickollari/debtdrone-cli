package tui

import (
	"context"
	"io"
	"log"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/endrilickollari/debtdrone-cli/internal/service"
	"github.com/endrilickollari/debtdrone-cli/internal/update"
)

var spinnerChars = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type scanCompleteMsg struct {
	path   string
	issues []models.TechnicalDebtIssue
	err    error
}

type scanProgressMsg struct {
	Task     string
	Progress float64
}

type checkUpdateMsg struct {
	info *update.UpdateInfo
	err  error
}

type updateCompleteMsg struct {
	err error
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second/10, func(t time.Time) tea.Msg { return tickMsg{} })
}

func startScan(path string, maxComplexity int, securityScan bool, progressChan chan tea.Msg) tea.Cmd {
	log.SetOutput(io.Discard)

	return func() tea.Msg {
		go func() {
			svc := service.NewScanService()
			ctx := context.Background()
			ctx = context.WithValue(ctx, "isCLI", true)

			opts := service.ScanOptions{
				MaxComplexity: maxComplexity,
				SecurityScan:  securityScan,
			}

			issues, err := svc.Run(ctx, path, opts, func(p service.ScanProgress) {
				progressChan <- scanProgressMsg{
					Task:     "Running " + p.AnalyzerName + "...",
					Progress: float64(p.Index) / float64(p.Total),
				}
				time.Sleep(300 * time.Millisecond)
			})

			if err != nil {
				log.SetOutput(os.Stderr)
				progressChan <- scanCompleteMsg{path: path, err: err}
				return
			}

			progressChan <- scanProgressMsg{
				Task:     "Finalizing results...",
				Progress: 1.0,
			}
			time.Sleep(500 * time.Millisecond)

			log.SetOutput(os.Stderr)
			progressChan <- scanCompleteMsg{path: path, issues: issues}
		}()
		return nil
	}
}

func startUpdateCheck() tea.Msg {
	ctx := context.Background()
	info, err := update.CheckForUpdate(ctx, version)
	if err != nil {
		return checkUpdateMsg{err: err}
	}
	return checkUpdateMsg{info: info}
}

func performUpdateCmd() tea.Msg {
	ctx := context.Background()
	err := update.PerformUpdate(ctx)
	return updateCompleteMsg{err: err}
}
