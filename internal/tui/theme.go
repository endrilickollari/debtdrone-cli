package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	colorAccentBlue = lipgloss.Color("#4fc3f7")
	colorDim        = lipgloss.Color("#4a5068")
	colorError      = lipgloss.Color("#ff5f5f")
	colorOK         = lipgloss.Color("#5af78e")
	colorCritical   = lipgloss.Color("#ff5f5f")
	colorHigh       = lipgloss.Color("#ffaa55")
	colorMedium     = lipgloss.Color("#ffd080")
	colorLow        = lipgloss.Color("#5a6080")
	colorFilePath   = lipgloss.Color("#8899bb")
	colorText       = lipgloss.Color("#c8d0e8")
	colorSelectedBg = lipgloss.Color("#1e2a40")
	colorBg         = lipgloss.Color("#1e2035")
)

func severityColor(sev string) lipgloss.Color {
	switch strings.ToLower(sev) {
	case "critical":
		return colorCritical
	case "high":
		return colorHigh
	case "medium":
		return colorMedium
	default:
		return colorLow
	}
}
