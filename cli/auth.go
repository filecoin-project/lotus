package cli

import (
	"fmt"

	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-jsonrpc/auth"

	"github.com/filecoin-project/lotus/api/apistruct"
	"github.com/filecoin-project/lotus/node/repo"
)

var authCmd = &cli.Command{
	Name:  "auth",
	Usage: "Manage RPC permissions",
	Subcommands: []*cli.Command{
		authCreateAdminToken,
		authApiInfoToken,
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
		napi, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if !cctx.IsSet("perm") {
			return xerrors.New("--perm flag not set")
		}

		perm := cctx.String("perm")
		idx := 0
		for i, p := range apistruct.AllPermissions {
			if auth.Permission(perm) == p {
				idx = i + 1
			}
		}

		if idx == 0 {
			return fmt.Errorf("--perm flag has to be one of: %s", apistruct.AllPermissions)
		}

		// slice on [:idx] so for example: 'sign' gives you [read, write, sign]
		token, err := napi.AuthNew(ctx, apistruct.AllPermissions[:idx])
		if err != nil {
			return err
		}

		// TODO: Log in audit log when it is implemented

		fmt.Println(string(token))
		return nil
	},
}

var authApiInfoToken = &cli.Command{
	Name:  "api-info",
	Usage: "Get token with API info required to connect to this node",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "perm",
			Usage: "permission to assign to the token, one of: read, write, sign, admin",
		},
	},

	Action: func(cctx *cli.Context) error {
		napi, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if !cctx.IsSet("perm") {
			return xerrors.New("--perm flag not set")
		}

		perm := cctx.String("perm")
		idx := 0
		for i, p := range apistruct.AllPermissions {
			if auth.Permission(perm) == p {
				idx = i + 1
			}
		}

		if idx == 0 {
			return fmt.Errorf("--perm flag has to be one of: %s", apistruct.AllPermissions)
		}

		// slice on [:idx] so for example: 'sign' gives you [read, write, sign]
		token, err := napi.AuthNew(ctx, apistruct.AllPermissions[:idx])
		if err != nil {
			return err
		}

		ti, ok := cctx.App.Metadata["repoType"]
		if !ok {
			log.Errorf("unknown repo type, are you sure you want to use GetAPI?")
			ti = repo.FullNode
		}
		t, ok := ti.(repo.RepoType)
		if !ok {
			log.Errorf("repoType type does not match the type of repo.RepoType")
		}

		ainfo, err := GetAPIInfo(cctx, t)
		if err != nil {
			return xerrors.Errorf("could not get API info: %w", err)
		}

		envVar := envForRepo(t)

		// TODO: Log in audit log when it is implemented

		fmt.Printf("%s=%s:%s\n", envVar, string(token), ainfo.Addr)
		return nil
	},
}
