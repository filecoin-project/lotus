package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/cli/lotus"
	"github.com/filecoin-project/lotus/cli/miner"
	"github.com/filecoin-project/lotus/cli/worker"
	"github.com/filecoin-project/lotus/node/config"
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

	cliApps := loadCLIApps()

	fmt.Println("Generating CLI documentation...")
	failed := generateCLIDocumentation(cliApps)

	fmt.Println("Generating default config files...")
	failed = generateDefaultConfigs() || failed

	if failed {
		fmt.Println("Documentation generation failed.")
		os.Exit(1)
	}
	fmt.Println("Documentation generation complete.")
}

func loadCLIApps() map[string]*cli.App {
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

	_ = w.Close()
	os.Stdout = stdout

	return cliApps
}

func generateCLIDocumentation(cliApps map[string]*cli.App) bool {
	var failed bool
	for name, app := range cliApps {
		resetCommandHelpName(app)

		generator := NewDocGenerator(outputDir, app)
		if err := generator.Generate(name); err != nil {
			fmt.Printf(" ❌ %s: %v\n", name, err)
			failed = true
			continue
		}
		fmt.Printf(" ✅ %s\n", name)
	}
	return failed
}

// resetCommandHelpName resets the HelpName of all commands to include the parent command names.
// This is needed for the case where Commands are shared between apps.
func resetCommandHelpName(app *cli.App) {
	var fix func(cmds []*cli.Command, helpName string)
	fix = func(cmds []*cli.Command, helpName string) {
		for _, cmd := range cmds {
			cmd.HelpName = fmt.Sprintf("%s %s", helpName, cmd.Name)
			fix(cmd.Subcommands, cmd.HelpName)
		}
	}
	fix(app.Commands, app.HelpName)
}

func generateDefaultConfigs() bool {
	var failed bool
	if err := generateDefaultConfig(config.DefaultFullNode(), "default-lotus-config.toml"); err != nil {
		fmt.Printf(" ❌ %s: %v\n", "lotus", err)
		failed = true
	} else {
		fmt.Printf(" ✅ %s\n", "lotus")
	}

	if err := generateDefaultConfig(config.DefaultStorageMiner(), "default-lotus-miner-config.toml"); err != nil {
		fmt.Printf(" ❌ %s: %v\n", "lotus-miner", err)
		failed = true
	} else {
		fmt.Printf(" ✅ %s\n", "lotus-miner")
	}
	return failed
}

func generateDefaultConfig(c interface{}, file string) error {
	cb, err := config.ConfigUpdate(c, nil, config.Commented(true), config.DefaultKeepUncommented())
	if err != nil {
		return err
	}
	output := filepath.Join(outputDir, file)
	return os.WriteFile(output, cb, 0644)
}
