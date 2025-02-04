package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	outputFormat string
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "text", "Output format (json|text)")
}

func rootCmd() *cobra.Command {
	var rootCmd = &cobra.Command{
		Use:   "lotus",
		Short: "Lotus CLI",
	}

	// Add subcommands here

	return rootCmd
}

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

var ChainReadObjCmd = &cobra.Command{
	Name:      "read-obj",
	Usage:     "Read the raw bytes of an object",
	ArgsUsage: "[objectCid]",
	Run: func(cmd *cobra.Command, args []string) {
		// ...

		switch outputFormat {
		case "json":
			// Output data in JSON format
			jsonData, _ := json.Marshal(data)
			fmt.Println(string(jsonData))
		case "text":
			// Output data in plain text format
			fmt.Println(data)
		default:
			return fmt.Errorf("unknown output format: %s", outputFormat)
		}

		return nil
	},
}

var ChainSetHeadCmd = &cobra.Command{
	Name:      "sethead",
	Aliases:   []string{"set-head"},
	Usage:     "manually set the local nodes head tipset (Caution: normally only used for recovery)",
	ArgsUsage: "[tipsetkey]",
	Run: func(cmd *cobra.Command, args []string) {
		// ...

		switch outputFormat {
		case "json":
			// Output data in JSON format
			jsonData, _ := json.Marshal(data)
			fmt.Println(string(jsonData))
		case "text":
			// Output data in plain text format
			fmt.Println(data)
		default:
			return fmt.Errorf("unknown output format: %s", outputFormat)
		}

		return nil
	},
}

// ... (update other subcommands similarly)

func printTipSet(format string, ts *types.TipSet, afmt *AppFmt) {
	// ...

	switch outputFormat {
	case "json":
		// Output data in JSON format
		jsonData, _ := json.Marshal(ts)
		fmt.Println(string(jsonData))
	case "text":
		// Output data in plain text format
		fmt.Println(ts)
	default:
		return fmt.Errorf("unknown output format: %s", outputFormat)
	}
}

func createExportFile(app *cli.App, path string) (io.WriteCloser, error) {
	// ...

	switch outputFormat {
	case "json":
		// Output data in JSON format
		jsonData, _ := json.Marshal(data)
		return os.Create(path + ".json"), nil
	case "text":
		// Output data in plain text format
		return os.Create(path + ".txt"), nil
	default:
		return nil, fmt.Errorf("unknown output format: %s", outputFormat)
	}
}

// Standardise a top level output format for all lotus CLIs