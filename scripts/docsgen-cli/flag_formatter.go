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

func (ff *FlagFormatter) Format() string {
	switch f := ff.flag.(type) {
	case *cli.StringFlag:
		return ff.formatWithEnv(strings.Join(f.Names(), ", "), f.Usage, f.Value, f.EnvVars)
	case *cli.BoolFlag:
		return ff.formatWithEnv(strings.Join(f.Names(), ", "), f.Usage, fmt.Sprintf("%t", f.Value), f.EnvVars)
	case *cli.IntFlag:
		return ff.formatWithEnv(strings.Join(f.Names(), ", "), f.Usage, fmt.Sprintf("%d", f.Value), f.EnvVars)
	default:
		names := make([]string, len(ff.flag.Names()))
		for i, n := range ff.flag.Names() {
			if len(n) == 1 {
				names[i] = "-" + n
			} else {
				names[i] = "--" + n
			}
		}
		return fmt.Sprintf("   %-30s %s\n", strings.Join(names, ", "), ff.flag.String())
	}
}

// formatWithEnv formats the flag string with optional environment variables
func (ff *FlagFormatter) formatWithEnv(name, usage, value string, envVars []string) string {
	// Get all names (including aliases)
	names := ff.flag.Names()

	// Format all names with proper prefix
	formattedNames := make([]string, len(names))
	for i, n := range names {
		if len(n) == 1 {
			// Short flag format: -v
			formattedNames[i] = "-" + n
		} else {
			// Long flag format: --verbose
			formattedNames[i] = "--" + n
		}
	}

	// Join all names with comma
	nameStr := strings.Join(formattedNames, ", ")

	const flagFormat = "   %-30s %s (default: \"%s\")"
	formatted := fmt.Sprintf(flagFormat, nameStr, usage, value)

	if len(envVars) > 0 {
		return formatted + fmt.Sprintf(" [%s]\n", strings.Join(envVars, ", "))
	}
	return formatted + "\n"
}
