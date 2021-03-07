package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/ipld/go-car"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var carCmd = &cli.Command{
	Name:        "car",
	Usage:       "CAR tools",
	Subcommands: []*cli.Command{
		carHeaderCmd,
	},
}

var carHeaderCmd = &cli.Command{
	Name: "header",
	Usage: "print CAR header info",
	ArgsUsage: "[car file]",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return xerrors.Errorf("expected 1 argument (car file)")
		}

		cf, err := os.Open(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("opening car file: %w", err)
		}
		defer cf.Close() // nolint

		h, l, err := car.ReadHeader(bufio.NewReader(cf))
		if err != nil {
			return xerrors.Errorf("reading car header: %w", err)
		}

		fmt.Printf("Length: %d bytes\n", l)
		fmt.Printf("Version: %d\n", h.Version)

		for i, root := range h.Roots {
			fmt.Printf("Root %d: %s\n", i, root)
		}

		return nil
	},
}