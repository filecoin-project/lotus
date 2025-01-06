package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/cli/lotus"
	"github.com/filecoin-project/lotus/cli/miner"
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

	// Some help output is generated based on whether the output is a terminal or not. To make stable
	// output text, we set Stdout to not be a terminal while we load the CLI apps and reset it
	// before generating the documentation.
	_, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w

	cliApps := map[string]*cli.App{
		"lotus":        lotus.App(),
		"lotus-worker": worker.App(),
		"lotus-miner":  miner.App(),
	}

	w.Close()
	os.Stdout = stdout

	fmt.Println("Generating CLI documentation...")

	for name, app := range cliApps {
		for _, cmd := range app.Commands {
			cmd.HelpName = fmt.Sprintf("%s %s", app.HelpName, cmd.Name)
		}

		generator := NewDocGenerator(outputDir, app)
		if err := generator.Generate(name); err != nil {
			fmt.Printf(" ❌ %s: %v\n", name, err)
			continue
		}
		fmt.Printf(" ✅ %s\n", name)
	}

	fmt.Println("Documentation generation complete.")
}
