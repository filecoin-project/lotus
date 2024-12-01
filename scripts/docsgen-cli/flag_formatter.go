package main

import (
	"fmt"
	"strings"

	"github.com/urfave/cli/v2"
)

// FlagFormatter handles flag formatting
type FlagFormatter struct {
	flag cli.Flag
}

// NewFlagFormatter creates a new flag formatter
func NewFlagFormatter(flag cli.Flag) *FlagFormatter {
	return &FlagFormatter{flag: flag}
}

// Format formats the flag string with optional environment variables
func (ff *FlagFormatter) Format() string {
	names, usage, value, envVars := ff.GetData()

	var formatted string
	if value != "" {
		formatted = fmt.Sprintf("   %-30s %s (default: %s)", names, usage, value)
	} else {
		formatted = fmt.Sprintf("   %-30s %s", names, usage)
	}

	if len(envVars) > 0 {
		return formatted + fmt.Sprintf(" [%s]\n", strings.Join(envVars, ", "))
	}
	return formatted + "\n"
}

func (ff *FlagFormatter) GetData() (string, string, string, []string) {
	switch f := ff.flag.(type) {
	case *cli.StringFlag:
		return ff.formatFlagNames(true), f.Usage, formatStringValue(f.Value), f.EnvVars
	case *cli.BoolFlag:
		return ff.formatFlagNames(false), f.Usage, formatBoolValue(f), f.EnvVars
	case *cli.IntFlag:
		return ff.formatFlagNames(true), f.Usage, fmt.Sprintf("%d", f.Value), f.EnvVars
	case *cli.Float64Flag:
		return ff.formatFlagNames(true), f.Usage, fmt.Sprintf("%g", f.Value), f.EnvVars
	case *cli.Int64Flag:
		return ff.formatFlagNames(true), f.Usage, fmt.Sprintf("%d", f.Value), f.EnvVars
	case *cli.UintFlag:
		return ff.formatFlagNames(true), f.Usage, fmt.Sprintf("%d", f.Value), f.EnvVars
	case *cli.Uint64Flag:
		return ff.formatFlagNames(true), f.Usage, fmt.Sprintf("%d", f.Value), f.EnvVars
	default:
		return ff.formatFlagNames(true), f.String(), "", nil
	}
}

// formatFlagNames formats all flag names and aliases
func (ff *FlagFormatter) formatFlagNames(showValue bool) string {
	names := ff.flag.Names()
	formattedNames := make([]string, len(names))

	for i, name := range names {
		if len(name) == 1 {
			formattedNames[i] = fmt.Sprintf("-%s", name) // Short flag
		} else {
			formattedNames[i] = fmt.Sprintf("--%s", name) // Long flag
		}

		if showValue && i == 0 {
			formattedNames[i] += " value"
		}
	}

	return strings.Join(formattedNames, ", ")
}
