package cli

import (
	"github.com/urfave/cli/v2"
)

var helloWorldCmd = &cli.Command{
	Name: "helloworld",
	Subcommands: []*cli.Command{
		helloHelloWorldCmd,
	},
}

var helloHelloWorldCmd = &cli.Command{
	Name: "hello",
	Action: func(cctx *cli.Context) error {
		apic, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if err := apic.Hello(ctx); err != nil {
			return err
		}

		return nil
	},
}
