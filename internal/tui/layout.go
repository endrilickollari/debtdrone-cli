package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Supported terminal size. Below either dimension the interface cannot lay out
// a readable screen, so it says so plainly instead of rendering a broken one.
// These values are published in the TUI documentation; keep the two in step.
const (
	minimumWidth  = 60
	minimumHeight = 16
)

// compactWidth is the breakpoint below which side-by-side panes stack and
// optional table columns are dropped. It is the narrowest width at which the
// dashboard's two panels still hold their content without wrapping.
const compactWidth = 96

// layout answers the questions the render functions ask about the terminal.
// Keeping the policy here means a breakpoint is defined once rather than
// rediscovered as a magic number in each screen.
type layout struct {
	width  int
	height int
}

func layoutFor(width, height int) layout {
	return layout{width: width, height: height}
}

// tooSmall reports a terminal that cannot hold a usable screen.
func (l layout) tooSmall() bool {
	return l.width < minimumWidth || l.height < minimumHeight
}

// compact reports a terminal narrow enough that panes should stack rather than
// sit side by side.
func (l layout) compact() bool {
	return l.width < compactWidth
}

// reducedMotion reports whether animation should be replaced with a static
// indicator. A terminal that cannot render colour is generally either a dumb
// terminal or a captured log, where a spinner is noise rather than feedback.
func reducedMotion() bool {
	return lipgloss.ColorProfile() == termenv.Ascii
}

// renderHints lays keyboard hints out across as many lines as the terminal
// requires. Hint rows are the widest fixed content on most screens, and a row
// that overflows is wrapped by lipgloss into a ragged line that displaces
// everything around it.
func renderHints(width int, segments []string) string {
	const separator = "   "

	var (
		lines   []string
		current string
	)
	for _, segment := range segments {
		candidate := segment
		if current != "" {
			candidate = current + separator + segment
		}
		if current != "" && lipgloss.Width(candidate) > width {
			lines = append(lines, current)
			current = segment
			continue
		}
		current = candidate
	}
	if current != "" {
		lines = append(lines, current)
	}

	for index, line := range lines {
		lines[index] = truncate(line, max(width, 1))
	}
	return lipgloss.NewStyle().Foreground(colorDim).Render(strings.Join(lines, "\n"))
}

// windowLines limits content to the rows available, scrolling so that the line
// at focus stays visible. A screen taller than its terminal is not merely ugly:
// the rows past the bottom edge cannot be read at all.
func windowLines(lines []string, height, focus int) []string {
	if height <= 0 {
		return nil
	}
	if len(lines) <= height {
		return lines
	}
	offset := clamp(focus-height/2, 0, len(lines)-height)
	return lines[offset : offset+height]
}

// windowLinesAt limits content using an explicit scroll offset. Unlike
// windowLines, it does not recenter around a selected row, which makes it fit
// read-only overlays such as help and persisted scan summaries.
func windowLinesAt(lines []string, height, offset int) []string {
	if height <= 0 {
		return nil
	}
	if len(lines) <= height {
		return lines
	}
	offset = clamp(offset, 0, len(lines)-height)
	return lines[offset : offset+height]
}

// renderTooSmall states the problem and what is required, rather than leaving
// the reader with a broken layout to interpret.
func renderTooSmall(width, height int) string {
	title := lipgloss.NewStyle().Foreground(colorHigh).Bold(true).Render("Terminal too small")
	detail := lipgloss.NewStyle().Foreground(colorText).Render(
		fmt.Sprintf("DebtDrone needs at least %d×%d. This terminal is %d×%d.",
			minimumWidth, minimumHeight, width, height))
	hint := lipgloss.NewStyle().Foreground(colorDim).Render("Resize the window, or press ctrl+c to quit.")

	body := lipgloss.JoinVertical(lipgloss.Left, title, "", detail, "", hint)
	// Placed against the origin rather than centred: centring inside a viewport
	// this small would push the message off screen.
	return lipgloss.NewStyle().MaxWidth(max(width, 1)).MaxHeight(max(height, 1)).Render(body)
}
