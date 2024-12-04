package main

import (
	"fmt"
	"strings"

	"github.com/urfave/cli/v2"
)

// formatCommandSummary formats a command summary with proper alignment.
func formatCommandSummary(cmd *cli.Command, maxWidth int) string {
	names := strings.Join(cmd.Names(), ", ")
	return fmt.Sprintf("   %-*s %s\n", maxWidth, names, cmd.Usage)
}

// calculateMaxCommandWidth determines the maximum width needed for command names
func calculateMaxCommandWidth(commands []*cli.Command, fixed int) int {
	maxWidth := 0
	for _, cmd := range commands {
		if width := len(strings.Join(cmd.Names(), ", ")); width > maxWidth {
			maxWidth = width
		}
	}
	return maxWidth + fixed
}
