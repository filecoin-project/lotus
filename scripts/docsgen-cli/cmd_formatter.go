package main

import (
	"fmt"
	"strings"

	"github.com/urfave/cli/v2"
)

// CommandFormatter handles command formatting
type CommandFormatter struct {
	cmd      *cli.Command
	rootName string
	depth    int
}

// NewCommandFormatter creates a new command formatter
func NewCommandFormatter(cmd *cli.Command, rootName string, depth int) *CommandFormatter {
	return &CommandFormatter{
		cmd:      cmd,
		rootName: rootName,
		depth:    depth,
	}
}

// Format formats a command's documentation
func (cf *CommandFormatter) Format() string {
	var sb strings.Builder
	cmdName := fmt.Sprintf("%s %s", cf.rootName, cf.cmd.Name)

	sb.WriteString(fmt.Sprintf("\n%s %s\n\n```\n", strings.Repeat("#", cf.depth+2), cmdName))

	// Command details
	sb.WriteString(cf.formatDetails(cmdName))

	// Subcommands
	if len(cf.cmd.Subcommands) > 0 {
		sb.WriteString("COMMANDS:\n")
		for _, subcmd := range cf.cmd.Subcommands {
			sb.WriteString(formatCommandSummary(subcmd))
		}

		sb.WriteString(helpCommandSummary())
	}

	sb.WriteString("```\n")
	return sb.String()
}

// formatDetails formats the detailed information of a command
func (cf *CommandFormatter) formatDetails(cmdName string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("NAME:\n   %s - %s\n\n", cmdName, cf.cmd.Usage))
	sb.WriteString(fmt.Sprintf("USAGE:\n   %s\n\n", generateUsage(cmdName, cf.cmd, cf.cmd.ArgsUsage)))

	if len(cf.cmd.Description) > 0 {
		sb.WriteString(fmt.Sprintf("DESCRIPTION:\n   %s\n\n", cf.cmd.Description))
	}

	if len(cf.cmd.Flags) > 0 {
		sb.WriteString("OPTIONS:\n")
		for _, flag := range cf.cmd.Flags {
			sb.WriteString(NewFlagFormatter(flag).Format())
		}
		// print help flag
		sb.WriteString(NewFlagFormatter(cli.HelpFlag).Format())
		sb.WriteString("\n")
	}

	return sb.String()
}
