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

// generateUsage dynamically creates a usage string for commands.
func generateUsage(parentName string) string {
	return fmt.Sprintf("%s [global options] command [command options] [arguments...]", parentName)
}

// formatStringValue formats default values properly
func formatStringValue(value string) string {
	if value == "" {
		return ""
	}
	return fmt.Sprintf("%q", value)
}

func formatBoolValue(f *cli.BoolFlag) string {
	if f.Name == "help" || f.Name == "version" {
		return ""
	}

	// if the flag is not set, and we have a default text, use it
	if !f.Value && f.GetDefaultText() != "" {
		return f.GetDefaultText()
	}

	return fmt.Sprintf("%t", f.Value)
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
