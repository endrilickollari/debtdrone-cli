package termsvg

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderProducesAStandaloneDocument(t *testing.T) {
	svg := string(Render("hello", DefaultOptions("Example screen")))

	assert.True(t, strings.HasPrefix(svg, "<svg xmlns=\"http://www.w3.org/2000/svg\""))
	assert.True(t, strings.HasSuffix(strings.TrimSpace(svg), "</svg>"))
	assert.Contains(t, svg, "<title>Example screen</title>")
	assert.Contains(t, svg, ">hello</text>")
}

func TestTrueColorForegroundAndBackgroundBecomeCSSColours(t *testing.T) {
	// Bright red text on a dark blue background, the shape lipgloss emits.
	screen := "\x1b[38;2;255;95;95m\x1b[48;2;30;42;64mcritical\x1b[0m"
	svg := string(Render(screen, DefaultOptions("colours")))

	assert.Contains(t, svg, `fill="#ff5f5f"`, "foreground colour is preserved")
	assert.Contains(t, svg, `fill="#1e2a40"`, "background colour becomes a rect")
	assert.Contains(t, svg, ">critical</text>")
}

func TestBoldBecomesAFontWeight(t *testing.T) {
	svg := string(Render("\x1b[1mScan failed\x1b[0m", DefaultOptions("bold")))
	assert.Contains(t, svg, `font-weight="700"`)
}

func TestResetEndsStylingSoLaterTextIsUnstyled(t *testing.T) {
	svg := string(Render("\x1b[38;2;255;0;0mred\x1b[0mplain", DefaultOptions("reset")))

	assert.Contains(t, svg, `fill="#ff0000"`)
	assert.Contains(t, svg, ">plain</text>")
	// The default foreground, not the red that preceded it.
	assert.Contains(t, svg, `fill="`+DefaultOptions("reset").Foreground+`">plain</text>`)
}

func TestEscapeSequencesNeverLeakIntoTheText(t *testing.T) {
	screen := "\x1b[38;2;1;2;3mstyled\x1b[0m plain \x1b[7mreverse\x1b[0m"
	svg := string(Render(screen, DefaultOptions("no leaks")))

	assert.NotContains(t, svg, "\x1b")
	assert.NotContains(t, svg, "38;2;1;2;3")
	assert.NotContains(t, svg, "[0m")
}

func TestMarkupInScreenContentIsEscaped(t *testing.T) {
	svg := string(Render(`<script>alert("x")</script> & more`, DefaultOptions("escaping")))

	assert.NotContains(t, svg, "<script>")
	assert.Contains(t, svg, "&lt;script&gt;")
	assert.Contains(t, svg, "&amp; more")
}

func TestLinesAreLaidOutOnSuccessiveRows(t *testing.T) {
	options := DefaultOptions("rows")
	svg := string(Render("first\nsecond", options))

	firstBaseline := options.Padding + options.FontSize*0.8
	secondBaseline := options.Padding + options.LineHeight + options.FontSize*0.8
	assert.Contains(t, svg, formatFloat(firstBaseline))
	assert.Contains(t, svg, formatFloat(secondBaseline))
}

func TestWideRunesAdvanceTwoColumns(t *testing.T) {
	options := DefaultOptions("width")
	// A full-width rune must push the following run two cells to the right, or
	// the text would drift out of its painted background.
	svg := string(Render("世x", options))

	require.Contains(t, svg, ">世x</text>")
	assert.Contains(t, svg, `viewBox="0 0 `+formatFloat(3*options.CellWidth+options.Padding*2))
}

func TestBlankScreenStillRendersAValidDocument(t *testing.T) {
	svg := string(Render("", DefaultOptions("blank")))
	assert.Contains(t, svg, "<svg")
	assert.Contains(t, svg, "</svg>")
}

// formatFloat matches the precision Render uses for coordinates.
func formatFloat(value float64) string {
	return fmt.Sprintf("%.1f", value)
}
