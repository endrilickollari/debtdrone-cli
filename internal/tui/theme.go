package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	colorAccentBlue = lipgloss.Color("#4fc3f7")
	colorDim        = lipgloss.Color("#949fb7")
	colorBorder     = lipgloss.Color("#687d97")
	colorError      = lipgloss.Color("#ff5f5f")
	colorOK         = lipgloss.Color("#5af78e")
	colorCritical   = lipgloss.Color("#ff5f5f")
	colorHigh       = lipgloss.Color("#ffaa55")
	colorMedium     = lipgloss.Color("#ffd080")
	colorLow        = lipgloss.Color("#aab4ce")
	colorFilePath   = lipgloss.Color("#8899bb")
	colorText       = lipgloss.Color("#c8d0e8")
	colorSelectedBg = lipgloss.Color("#26354f")
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
