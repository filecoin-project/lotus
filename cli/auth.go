package cli

import (
	"errors"
	"fmt"

	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/api"
)

var authCmd = &cli.Command{
	Name:  "auth",
	Usage: "Manage RPC permissions",
	Subcommands: []*cli.Command{
		authCreateAdminToken,
	},
}

var authCreateAdminToken = &cli.Command{
	Name:  "create-token",
	Usage: "Create token",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "perm",
			Usage: "permission to assign to the token, one of: read, write, sign, admin",
		},
	},

	Action: func(cctx *cli.Context) error {
		napi, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if !cctx.IsSet("perm") {
			return errors.New("--perm flag not set")
		}

		perm := cctx.String("perm")
		idx := -1
		for i, p := range api.AllPermissions {
			if perm == p {
				idx = i
			}
		}

		if idx == -1 {
			return fmt.Errorf("--perm flag has to be one of: %s", api.AllPermissions)
		}

		// TODO: Probably tell the user how powerful this token is

		token, err := napi.AuthNew(ctx, api.AllPermissions[:idx])
		if err != nil {
			return err
		}

		// TODO: Log in audit log when it is implemented

		fmt.Println(string(token))
		return nil
	},
}
