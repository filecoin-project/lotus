package cli

import (
	"fmt"

	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-lotus/api"
)

var authCmd = &cli.Command{
	Name:  "auth",
	Usage: "Manage RPC permissions",
	Subcommands: []*cli.Command{
		authCreateAdminToken,
	},
}

var authCreateAdminToken = &cli.Command{
	Name:  "create-admin-token",
	Usage: "Create admin token",
	Action: func(cctx *cli.Context) error {
		napi, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		ctx := ReqContext(cctx)

		// TODO: Probably tell the user how powerful this token is

		token, err := napi.AuthNew(ctx, api.AllPermissions)
		if err != nil {
			return err
		}

		// TODO: Log in audit log when it is implemented

		fmt.Println(string(token))
		return nil
	},
}
