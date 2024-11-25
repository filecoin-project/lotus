package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"

	"github.com/filecoin-project/lotus/cli/worker"
)

const (
	outputDir           = "documentation/en"
	depthRecursionLimit = 1024
)

func main() {
	// Unset environment variables to avoid interference.
	for _, e := range []string{
		"LOTUS_PATH",
		"LOTUS_MINER_PATH",
		"LOTUS_STORAGE_PATH",
		"LOTUS_WORKER_PATH",
		"LOTUS_PANIC_REPORT_PATH",
		"WALLET_PATH",
		"WORKER_PATH",
	} {
		if err := os.Unsetenv(e); err != nil {
			fmt.Printf("Failed to unset %s: %v\n", e, err)
			os.Exit(1)
		}
	}

	// Skip commit suffix in version commands.
	if err := os.Setenv("LOTUS_VERSION_IGNORE_COMMIT", "1"); err != nil {
		fmt.Printf("Failed to set LOTUS_VERSION_IGNORE_COMMIT: %v\n", err)
		os.Exit(1)
	}

	//  Make sure output directory exists.
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		fmt.Printf("Failed to create %s: %v\n", outputDir, err)
		os.Exit(1)
	}

	fmt.Println("Generating CLI documentation...")
	var eg errgroup.Group
	// Add CLI apps for documentation generation
	cliApps := map[string]*cli.App{
		"lotus-worker": worker.App(),
		// Add other CLI apps like lotus, lotus-miner as needed
	}

	for name, app := range cliApps {
		eg.Go(func() error {
			err := generateDocsFromApp(name, app)
			if err != nil {
				fmt.Printf(" ❌ %s: %v\n", name, err)
			} else {
				fmt.Printf(" ✅ %s\n", name)
			}
			return err
		})
	}

	if err := eg.Wait(); err != nil {
		fmt.Printf("Failed to generate CLI documentation: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Documentation generation complete.")
}

func generateDocsFromApp(name string, app *cli.App) error {
	md := filepath.Join(outputDir, fmt.Sprintf("cli-%s.md", name))
	out, err := os.Create(md)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	// Write top-level documentation
	if _, err := out.WriteString(formatAppHeader(app)); err != nil {
		return err
	}

	return writeDocs(out, app.Commands, 0, name)
}

// formatAppHeader dynamically generates the header for the CLI app.
func formatAppHeader(app *cli.App) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", app.Name))
	sb.WriteString("```\n")
	sb.WriteString(fmt.Sprintf("NAME:\n   %s - %s\n\n", app.Name, app.Usage))
	sb.WriteString(fmt.Sprintf("USAGE:\n   %s\n\n", generateUsage(app.Name, nil, ""))) // Dynamically fetch usage text
	sb.WriteString(fmt.Sprintf("VERSION:\n   %s\n\n", app.Version))

	sb.WriteString("COMMANDS:\n")
	for _, cmd := range app.Commands {
		sb.WriteString(fmt.Sprintf("   %-10s %s\n", cmd.Name, cmd.Usage))
	}

	sb.WriteString("\nGLOBAL OPTIONS:\n")
	for _, flag := range app.Flags {
		sb.WriteString(formatFlag(flag))
	}
	sb.WriteString("```\n")
	return sb.String()
}

// formatFlag formats a cli.Flag into a string for documentation.
func formatFlag(flag cli.Flag) string {
	switch f := flag.(type) {
	case *cli.StringFlag:
		return fmt.Sprintf("   --%-20s %s (default: \"%s\") [%s]\n", f.Name, f.Usage, f.Value, strings.Join(f.EnvVars, ", "))
	case *cli.BoolFlag:
		return fmt.Sprintf("   --%-20s %s (default: %t) [%s]\n", f.Name, f.Usage, f.Value, strings.Join(f.EnvVars, ", "))
	case *cli.IntFlag:
		return fmt.Sprintf("   --%-20s %s (default: %d) [%s]\n", f.Name, f.Usage, f.Value, strings.Join(f.EnvVars, ", "))
	default:
		return fmt.Sprintf("   --%-20s %s\n", flag.Names()[0], flag.String())
	}
}

// generateUsage creates the `USAGE` dynamically for root or subcommands.
func generateUsage(parentName string, cmd *cli.Command, argsUsage string) string {
	if cmd == nil {
		// Root command usage
		return fmt.Sprintf("%s [global options] command [command options] [arguments...]", parentName)
	}

	usage := ""
	if len(cmd.Subcommands) > 0 {
		usage = fmt.Sprintf("%s %s command [command options]", parentName, cmd.Name)
	} else {
		usage = fmt.Sprintf("%s %s [command options] ", parentName, cmd.Name)
	}

	if argsUsage != "" {
		usage += " " + argsUsage
	} else {
		usage += " [arguments...]"
	}

	return usage
}

func writeDocs(file *os.File, commands []*cli.Command, depth int, rootCommandName string) error {
	for _, cmd := range commands {
		cmdName := cmd.Name
		if rootCommandName != cmdName {
			cmdName = rootCommandName + " " + cmdName
		}

		header := fmt.Sprintf("\n%s %s\n\n", strings.Repeat("#", depth+2), cmdName)
		if _, err := file.WriteString(header); err != nil {
			return err
		}

		if _, err := file.WriteString("```\n"); err != nil {
			return err
		}

		// Write command details
		if _, err := file.WriteString(fmt.Sprintf("NAME:\n   %s - %s\n\n", cmdName, cmd.Usage)); err != nil {
			return err
		}

		if _, err := file.WriteString(fmt.Sprintf("USAGE:\n   %s\n\n", generateUsage(rootCommandName, cmd, cmd.ArgsUsage))); err != nil {
			return err
		}

		if len(cmd.Description) > 0 {
			if _, err := file.WriteString(fmt.Sprintf("DESCRIPTION:\n   %s\n\n", cmd.Description)); err != nil {
				return err
			}
		}

		// Write options for the command
		if len(cmd.Flags) > 0 {
			if _, err := file.WriteString("OPTIONS:\n"); err != nil {
				return err
			}
			for _, flag := range cmd.Flags {
				if _, err := file.WriteString(formatFlag(flag)); err != nil {
					return err
				}
			}
			if _, err := file.WriteString("\n"); err != nil {
				return err
			}
		}

		if len(cmd.Subcommands) > 0 {
			if _, err := file.WriteString("COMMANDS:\n"); err != nil {
				return err
			}

			for _, subcmd := range cmd.Subcommands {
				if _, err := file.WriteString(fmt.Sprintf("   %-10s %s\n", subcmd.Name, subcmd.Usage)); err != nil {
					return err
				}
			}

			if _, err := file.WriteString("```\n"); err != nil {
				return err
			}

			if err := writeDocs(file, cmd.Subcommands, depth+1, cmdName); err != nil {
				return err
			}

			continue
		}

		if _, err := file.WriteString("```\n"); err != nil {
			return err
		}
	}

	return nil
}
