// Package termsvg renders ANSI-styled terminal output as a standalone SVG.
//
// It exists so documentation images can be produced from the same screens the
// tests render, making an asset refresh a repeatable command rather than a
// manual screenshot. It understands only the styling DebtDrone's TUI emits:
// reset, bold, and 24-bit foreground and background colours.
package termsvg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
)

// Options control the rendered image. The zero value is not useful; use
// DefaultOptions and adjust from there.
type Options struct {
	// Title is announced to assistive technology and shown by some viewers.
	Title string
	// FontFamily is the CSS font stack used for every cell.
	FontFamily string
	// FontSize is the glyph size in pixels.
	FontSize float64
	// CellWidth and LineHeight are the advance per column and per row. They
	// must match the font's metrics or text will drift from its background.
	CellWidth  float64
	LineHeight float64
	// Padding is the margin between the terminal content and the image edge.
	Padding float64
	// Background is the page colour behind every cell.
	Background string
	// Foreground is the colour of text that carries no explicit colour.
	Foreground string
}

// DefaultOptions matches the dark palette the TUI is designed against. The
// cell metrics correspond to the bundled Geist Mono face at 14px.
func DefaultOptions(title string) Options {
	return Options{
		Title:      title,
		FontFamily: "'Geist Mono', 'SFMono-Regular', 'JetBrains Mono', Menlo, Consolas, monospace",
		FontSize:   14,
		CellWidth:  8.4,
		LineHeight: 18,
		Padding:    16,
		Background: "#12141f",
		Foreground: "#c8d0e8",
	}
}

// style is the resolved appearance of one run of cells.
type style struct {
	foreground string
	background string
	bold       bool
}

// cell is one terminal column: its rune and the style in effect for it.
type cell struct {
	content rune
	style   style
}

// Render converts ANSI-styled terminal output into a complete SVG document.
func Render(screen string, options Options) []byte {
	lines := parseScreen(screen)

	columns := 0
	for _, line := range lines {
		width := 0
		for _, c := range line {
			width += runewidth.RuneWidth(c.content)
		}
		if width > columns {
			columns = width
		}
	}

	imageWidth := float64(columns)*options.CellWidth + options.Padding*2
	imageHeight := float64(len(lines))*options.LineHeight + options.Padding*2

	var out bytes.Buffer
	fmt.Fprintf(&out, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.1f %.1f" width="%.1f" height="%.1f" role="img" aria-label="%s">`,
		imageWidth, imageHeight, imageWidth, imageHeight, escape(options.Title))
	fmt.Fprintf(&out, `<title>%s</title>`, escape(options.Title))
	fmt.Fprintf(&out, `<rect width="%.1f" height="%.1f" rx="8" fill="%s"/>`,
		imageWidth, imageHeight, options.Background)

	// Backgrounds are painted for every line first so a coloured run can never
	// be drawn over the glyphs of the line above it.
	for row, line := range lines {
		writeBackgrounds(&out, line, row, options)
	}
	fmt.Fprintf(&out, `<g font-family="%s" font-size="%.1fpx" xml:space="preserve">`,
		escape(options.FontFamily), options.FontSize)
	for row, line := range lines {
		writeText(&out, line, row, options)
	}
	out.WriteString(`</g></svg>`)
	out.WriteString("\n")
	return out.Bytes()
}

func writeBackgrounds(out *bytes.Buffer, line []cell, row int, options Options) {
	column := 0
	for index := 0; index < len(line); {
		current := line[index].style.background
		runStart, runWidth := column, 0
		for index < len(line) && line[index].style.background == current {
			width := runewidth.RuneWidth(line[index].content)
			runWidth += width
			column += width
			index++
		}
		if current == "" || runWidth == 0 {
			continue
		}
		fmt.Fprintf(out, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`,
			options.Padding+float64(runStart)*options.CellWidth,
			options.Padding+float64(row)*options.LineHeight,
			float64(runWidth)*options.CellWidth,
			options.LineHeight,
			current)
	}
}

func writeText(out *bytes.Buffer, line []cell, row int, options Options) {
	// The baseline sits proportionally within the line box so descenders are
	// not clipped by the following row's background.
	baseline := options.Padding + float64(row)*options.LineHeight + options.FontSize*0.8

	column := 0
	for index := 0; index < len(line); {
		current := line[index].style
		runStart := column
		var text strings.Builder
		for index < len(line) && line[index].style == current {
			text.WriteRune(line[index].content)
			column += runewidth.RuneWidth(line[index].content)
			index++
		}
		if strings.TrimSpace(text.String()) == "" {
			continue
		}

		fill := current.foreground
		if fill == "" {
			fill = options.Foreground
		}
		weight := ""
		if current.bold {
			weight = ` font-weight="700"`
		}
		fmt.Fprintf(out, `<text x="%.1f" y="%.1f" fill="%s"%s>%s</text>`,
			options.Padding+float64(runStart)*options.CellWidth, baseline, fill, weight, escape(text.String()))
	}
}

// parseScreen turns ANSI-styled text into styled cells, one slice per line.
func parseScreen(screen string) [][]cell {
	var lines [][]cell
	for _, raw := range strings.Split(strings.TrimRight(screen, "\n"), "\n") {
		lines = append(lines, parseLine(raw))
	}
	return lines
}

func parseLine(raw string) []cell {
	var (
		cells   []cell
		current style
		runes   = []rune(raw)
	)

	for index := 0; index < len(runes); {
		if runes[index] == 0x1b && index+1 < len(runes) && runes[index+1] == '[' {
			consumed, updated, ok := parseSGR(runes[index:], current)
			if ok {
				current = updated
				index += consumed
				continue
			}
		}
		if runes[index] == '\r' {
			index++
			continue
		}
		cells = append(cells, cell{content: runes[index], style: current})
		index++
	}
	return cells
}

// parseSGR consumes one CSI sequence and applies it. Sequences that are not
// SGR are consumed and ignored so their parameters never leak into the text.
func parseSGR(runes []rune, current style) (consumed int, updated style, ok bool) {
	end := 2
	for end < len(runes) && runes[end] != 'm' && !isCSIFinal(runes[end]) {
		end++
	}
	if end >= len(runes) {
		return 0, current, false
	}
	if runes[end] != 'm' {
		return end + 1, current, true
	}

	params := parseParams(string(runes[2:end]))
	for index := 0; index < len(params); index++ {
		switch code := params[index]; {
		case code == 0:
			current = style{}
		case code == 1:
			current.bold = true
		case code == 22:
			current.bold = false
		case code == 39:
			current.foreground = ""
		case code == 49:
			current.background = ""
		case code == 38 || code == 48:
			colour, used := parseExtendedColour(params[index:])
			if used == 0 {
				// An unrecognised colour form: stop rather than misread the
				// remaining parameters as separate codes.
				index = len(params)
				break
			}
			if code == 38 {
				current.foreground = colour
			} else {
				current.background = colour
			}
			index += used - 1
		}
	}
	return end + 1, current, true
}

// parseExtendedColour reads a 38/48 colour parameter, returning the CSS colour
// and how many parameters it consumed.
func parseExtendedColour(params []int) (string, int) {
	if len(params) >= 5 && params[1] == 2 {
		return fmt.Sprintf("#%02x%02x%02x", clampByte(params[2]), clampByte(params[3]), clampByte(params[4])), 5
	}
	if len(params) >= 3 && params[1] == 5 {
		return ansi256(params[2]), 3
	}
	return "", 0
}

func parseParams(raw string) []int {
	if raw == "" {
		return []int{0}
	}
	fields := strings.Split(raw, ";")
	params := make([]int, 0, len(fields))
	for _, field := range fields {
		if field == "" {
			params = append(params, 0)
			continue
		}
		value, err := strconv.Atoi(field)
		if err != nil {
			return nil
		}
		params = append(params, value)
	}
	return params
}

// ansi256 maps an 8-bit palette index onto a CSS colour. The TUI renders at
// TrueColor, so this covers terminals that degrade rather than normal output.
func ansi256(index int) string {
	switch {
	case index < 0 || index > 255:
		return ""
	case index < 16:
		base := []string{
			"#000000", "#800000", "#008000", "#808000", "#000080", "#800080", "#008080", "#c0c0c0",
			"#808080", "#ff0000", "#00ff00", "#ffff00", "#0000ff", "#ff00ff", "#00ffff", "#ffffff",
		}
		return base[index]
	case index < 232:
		steps := []int{0, 95, 135, 175, 215, 255}
		value := index - 16
		return fmt.Sprintf("#%02x%02x%02x", steps[value/36], steps[(value/6)%6], steps[value%6])
	default:
		grey := 8 + (index-232)*10
		return fmt.Sprintf("#%02x%02x%02x", grey, grey, grey)
	}
}

func clampByte(value int) int {
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return value
}

func isCSIFinal(r rune) bool {
	return r >= 0x40 && r <= 0x7e
}

func escape(text string) string {
	var out bytes.Buffer
	_ = xml.EscapeText(&out, []byte(text))
	return out.String()
}
