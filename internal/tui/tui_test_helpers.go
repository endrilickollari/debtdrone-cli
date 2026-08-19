package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func init() {
	// Force TrueColor profile to prevent CI test flakes due to color degradation
	lipgloss.SetColorProfile(termenv.TrueColor)
}

func keyMsg(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{
		Code: r,
		Text: string(r),
	}
}

func specialKeyMsg(k rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{
		Code: k,
	}
}
