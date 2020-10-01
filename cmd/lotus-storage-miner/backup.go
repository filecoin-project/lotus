package main

import (
	"fmt"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	lcli "github.com/filecoin-project/lotus/cli"
)

var backupCmd = &cli.Command{
	Name:  "backup",
	Usage: "Create node metadata backup",
	Description: `The backup command writes a copy of node metadata under the specified path

For security reasons, the daemon must be have LOTUS_BACKUP_BASE_PATH env var set
to a path where backup files are supposed to be saved, and the path specified in
this command must be within this base path`,
	ArgsUsage: "[backup file path]",
	Flags:     []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		if cctx.Args().Len() != 1 {
			return xerrors.Errorf("expected 1 argument")
		}

		err = api.CreateBackup(lcli.ReqContext(cctx), cctx.Args().First())
		if err != nil {
			return err
		}

		fmt.Println("Success")

		return nil
	},
}
