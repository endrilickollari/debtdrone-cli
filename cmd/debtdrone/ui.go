package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/schollz/progressbar/v3"
)

const DroneBanner = `                                  
.--------------------------------------------------------------------------------------.
|                                                                                      |
|                                                                                      |
|                                                                                      |
|                                                                                      |
|    .______  ._______._______ _____._.______  .______  ._______  .______  ._______    |
|    :_ _   \ : .____/: __   / \__ _:|:_ _   \ : __   \ : .___  \ :      \ : .____/    |
|    |   |   || : _/\ |  |>  \   |  :||   |   ||  \____|| :   |  ||       || : _/\     |
|    | . |   ||   /  \|  |>   \  |   || . |   ||   :  \ |     :  ||   |   ||   /  \    |
|    |. ____/ |_.: __/|_______/  |   ||. ____/ |   |___\ \_. ___/ |___|   ||_.: __/    |
|     :/         :/              |___| :/      |___|       :/         |___|   :/       |
|     :                                :                   :                           |
|                                                                                      |
|                                                                                      |
|                                                                                      |
|                                                                                      |
'--------------------------------------------------------------------------------------'
`

func printBanner() {
	cyan := color.New(color.FgCyan).SprintFunc()
	banner := strings.TrimPrefix(DroneBanner, "\n")

	fmt.Println()
	fmt.Println(cyan(banner))
	fmt.Println()
	fmt.Println()
}

func startSpinner(max int, description string) *progressbar.ProgressBar {
	bar := progressbar.NewOptions(max,
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetWriter(color.Output),
		progressbar.OptionSetWidth(15),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionOnCompletion(func() {
			fmt.Println()
		}),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
	)
	return bar
}
