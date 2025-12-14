package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/schollz/progressbar/v3"
)

const DroneBanner = `                                  
░█▀▄░█▀▀░█▀▄░▀█▀░█▀▄░█▀▄░█▀█░█▀█░█▀▀
░█░█░█▀▀░█▀▄░░█░░█░█░█▀▄░█░█░█░█░█▀▀
░▀▀░░▀▀▀░▀▀░░░▀░░▀▀░░▀░▀░▀▀▀░▀░▀░▀▀▀
`

func printBanner() {
	cyan := color.New(color.FgCyan).SprintFunc()
	banner := strings.TrimPrefix(DroneBanner, "\n")

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, cyan(banner))
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr)
}

func startSpinner(max int, description string) *progressbar.ProgressBar {
	bar := progressbar.NewOptions(max,
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetWidth(15),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprintln(os.Stderr)
		}),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
	)
	return bar
}
