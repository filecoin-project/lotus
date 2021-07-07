package cliutil

import (
	"os"

	"github.com/mattn/go-isatty"
)

var DefaultColorUse = isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
