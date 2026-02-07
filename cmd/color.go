package cmd

import (
	"os"

	"github.com/fatih/color"
)

var (
	colorRed    = color.New(color.FgRed).SprintFunc()
	colorGreen  = color.New(color.FgGreen).SprintFunc()
	colorYellow = color.New(color.FgYellow).SprintFunc()
	colorCyan   = color.New(color.FgCyan).SprintFunc()
	colorBold   = color.New(color.Bold).SprintFunc()
)

func initColor() {
	if noColor || os.Getenv("NO_COLOR") != "" {
		color.NoColor = true
	}
}
