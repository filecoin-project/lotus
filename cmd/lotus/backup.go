package main

import (
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-jsonrpc"

	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/node/repo"
)

var backupCmd = lcli.BackupCmd("repo", repo.FullNode, func(cctx *cli.Context) (lcli.BackupAPI, jsonrpc.ClientCloser, error) {
	return lcli.GetFullNodeAPI(cctx)
})
