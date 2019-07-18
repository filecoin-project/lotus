package cli

import (
	"fmt"

	"gopkg.in/urfave/cli.v2"
)

var versionCmd = &cli.Command{
	Name:  "version",
	Usage: "Print version",
	Action: func(cctx *cli.Context) error {
		api, err := GetAPI(cctx)
		if err != nil {
			return err
		}

		ctx := ReqContext(cctx)
		// TODO: print more useful things

		fmt.Println(api.Version(ctx))
		cli.VersionPrinter(cctx)
		return nil
	},
}
