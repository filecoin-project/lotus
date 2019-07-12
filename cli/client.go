package cli

import (
	"fmt"
	"gopkg.in/urfave/cli.v2"
)

var clientCmd = &cli.Command{
	Name:  "client",
	Usage: "Make deals, store data, retrieve data",
	Subcommands: []*cli.Command{
		clientImportCmd,
	},
}

var clientImportCmd = &cli.Command{
	Name: "import",
	Usage: "import data",
	Action: func(cctx *cli.Context) error {
		api, err := getAPI(cctx)
		if err != nil {
			return err
		}
		ctx := reqContext(cctx)

		c, err := api.ClientImport(ctx, cctx.Args().First())
		if err != nil {
			return err
		}
		fmt.Println(c.String())
		return nil
	},
}