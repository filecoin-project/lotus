package cli

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/build"
)

var VersionCmd = &cli.Command{
	Name:  "version",
	Usage: "Print version",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "commit",
			Usage:   "Output git commit information",
			Aliases: []string{"c"},
		},
		&cli.BoolFlag{
			Name:    "build-info",
			Usage:   "Output all build information",
			Aliases: []string{"b"},
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetAPI(cctx)
		if err != nil {
			fmt.Printf("Getting remote API: %s (daemon not running?)\n", err)
		} else {
			defer closer()

			ctx := ReqContext(cctx)
			// TODO: print more useful things

			v, err := api.Version(ctx)
			if err != nil {
				return err
			}
			fmt.Println("Daemon: ", v)
		}

		fmt.Print("Local: ")
		cli.VersionPrinter(cctx)

		if cctx.Bool("commit") || cctx.Bool("build-info") {
			bi, err := build.Build()
			if err != nil {
				return err
			}

			fmt.Println()
			fmt.Println(bi.GitHead)
		}

		if cctx.Bool("build-info") {
			bi, err := build.Build()
			if err != nil {
				return err
			}

			fmt.Println(bi.GitStatus)

			fmt.Println("-----")
			fmt.Println("go.mod:")
			fmt.Println(bi.GoMod)
		}

		return nil
	},
}
