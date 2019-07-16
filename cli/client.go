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
		clientLocalCmd,
	},
}

var clientImportCmd = &cli.Command{
	Name:  "import",
	Usage: "Import data",
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

var clientLocalCmd = &cli.Command{
	Name:  "local",
	Usage: "List locally imported data",
	Action: func(cctx *cli.Context) error {
		api, err := getAPI(cctx)
		if err != nil {
			return err
		}
		ctx := reqContext(cctx)

		list, err := api.ClientListImports(ctx)
		if err != nil {
			return err
		}
		for _, v := range list {
			fmt.Printf("%s %s %d %s\n", v.Key, v.FilePath, v.Size, v.Status)
		}
		return nil
	},
}
