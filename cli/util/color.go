package cliutil

import (
	"os"

	"github.com/mattn/go-isatty"
)

// DefaultColorUse is the globally referenced variable for all Lotus CLI tools
// It sets to `true` if STDOUT is a TTY or if the variable GOLOG_LOG_FMT is set
// to color (as recognizd by github.com/ipfs/go-log/v2)
var DefaultColorUse = os.Getenv("GOLOG_LOG_FMT") == "color" ||
	isatty.IsTerminal(os.Stdout.Fd()) ||
	isatty.IsCygwinTerminal(os.Stdout.Fd())
