package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"
)

var root []*cli.Command = []*cli.Command{
	createSimCommand,
	deleteSimCommand,
	copySimCommand,
	renameSimCommand,
	listSimCommand,

	runSimCommand,
	infoSimCommand,
	upgradeCommand,
}

func main() {
	if _, set := os.LookupEnv("GOLOG_LOG_LEVEL"); !set {
		_ = logging.SetLogLevel("simulation", "DEBUG")
		_ = logging.SetLogLevel("simulation-mock", "DEBUG")
	}
	app := &cli.App{
		Name:      "lotus-sim",
		Usage:     "A tool to simulate a network.",
		Commands:  root,
		Writer:    os.Stdout,
		ErrWriter: os.Stderr,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PATH"},
				Hidden:  true,
				Value:   "~/.lotus",
			},
			&cli.StringFlag{
				Name:    "simulation",
				Aliases: []string{"sim"},
				EnvVars: []string{"LOTUS_SIMULATION"},
				Value:   "default",
			},
		},
	}

	ctx, cancel := signal.NotifyContext(context.Background(),
		syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer cancel()

	if err := app.RunContext(ctx, os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
		return
	}
}
