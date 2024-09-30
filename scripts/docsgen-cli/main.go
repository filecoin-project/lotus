package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sync/errgroup"
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
	for _, cmd := range []string{"lotus", "lotus-miner", "lotus-worker"} {
		eg.Go(func() error {
			err := generateMarkdownForCLI(cmd)
			if err != nil {
				fmt.Printf(" ❌ %s: %v\n", cmd, err)
			} else {
				fmt.Printf(" ✅ %s\n", cmd)
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

func generateMarkdownForCLI(cli string) error {
	md := filepath.Join(outputDir, fmt.Sprintf("cli-%s.md", cli))
	out, err := os.Create(md)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	return writeCommandDocs(out, cli, 0)
}

func writeCommandDocs(file *os.File, command string, depth int) error {
	// For sanity, fail fast if depth exceeds some arbitrarily large number. In which
	// case, chances are there is a bug in this script.
	if depth > depthRecursionLimit {
		return fmt.Errorf("recursion exceeded limit of %d", depthRecursionLimit)
	}

	// Get usage from the command.
	usage, err := exec.Command("sh", "-c", "./"+command+" -h").Output()
	if err != nil {
		return fmt.Errorf("failed to run '%s': %v", command, err)
	}

	// Skip the first new line since the docs do not start with a newline at the very
	// top.
	if depth != 0 {
		if _, err := file.WriteString("\n"); err != nil {
			return err
		}
	}

	// Write out command header and usage.
	header := fmt.Sprintf("%s# %s\n", strings.Repeat("#", depth), command)
	if _, err := file.WriteString(header); err != nil {
		return err
	} else if _, err := file.WriteString("```\n"); err != nil {
		return err
	} else if _, err := file.Write(usage); err != nil {
		return err
	} else if _, err := file.WriteString("```\n"); err != nil {
		return err
	}

	// Recurse sub-commands.
	commands := false
	lines := strings.Split(string(usage), "\n")
	for _, line := range lines {
		switch line = strings.TrimSpace(line); {
		case line == "":
			commands = false
		case line == "COMMANDS:":
			commands = true
		case strings.HasPrefix(line, "help, h"):
			// Skip usage command.
		case commands:
			// Find the sub command and trim any potential comma in case of alias.
			subCommand := strings.TrimSuffix(strings.Fields(line)[0], ",")
			// Skip sections in usage that have no command.
			if !strings.Contains(subCommand, ":") {
				if err := writeCommandDocs(file, command+" "+subCommand, depth+1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
