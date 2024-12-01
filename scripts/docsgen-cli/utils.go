package main

import (
	"fmt"
	"strings"

	"github.com/urfave/cli/v2"
)

func formatCommandSummary(cmd *cli.Command) string {
	names := strings.Join(cmd.Names(), ", ")
	return fmt.Sprintf("   %-30s %s\n", names, cmd.Usage)
}

// generateUsage creates a usage string dynamically for root or subcommands.
func generateUsage(parentName string, cmd *cli.Command, argsUsage string) string {
	if cmd == nil {
		return fmt.Sprintf("%s [global options] command [command options] [arguments...]", parentName)
	}

	usage := parentName
	if len(cmd.Subcommands) > 0 {
		usage += " command [command options]"
	} else {
		usage += " [command options]"
	}

	if argsUsage != "" {
		usage += " " + argsUsage
	} else {
		usage += " [arguments...]"
	}

	return usage
}

// helpCommandSummary formats the help command summary
func helpCommandSummary() string {
	return formatCommandSummary(&cli.Command{
		Name:    "help",
		Aliases: []string{"h"},
		Usage:   "Shows a list of commands or help for one command",
	})
}

// formatDefaultValue formats default values properly
func formatDefaultValue(value string) string {
	if value == "" {
		return ""
	}
	return fmt.Sprintf("%q", value)
}
