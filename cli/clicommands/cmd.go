package clicommands

import (
	"github.com/urfave/cli/v2"

	lcli "github.com/filecoin-project/lotus/cli"
)

var Commands = []*cli.Command{
	lcli.WithCategory("basic", lcli.SendCmd),
	lcli.WithCategory("basic", lcli.WalletCmd),
	lcli.WithCategory("basic", lcli.InfoCmd),
	lcli.WithCategory("basic", lcli.MultisigCmd),
	lcli.WithCategory("basic", lcli.FilplusCmd),
	lcli.WithCategory("basic", lcli.PaychCmd),
	lcli.WithCategory("developer", lcli.AuthCmd),
	lcli.WithCategory("developer", lcli.MpoolCmd),
	lcli.WithCategory("developer", StateCmd),
	lcli.WithCategory("developer", lcli.ChainCmd),
	lcli.WithCategory("developer", lcli.LogCmd),
	lcli.WithCategory("developer", lcli.WaitApiCmd),
	lcli.WithCategory("developer", lcli.FetchParamCmd),
	lcli.WithCategory("developer", lcli.EvmCmd),
	lcli.WithCategory("developer", lcli.IndexCmd),
	lcli.WithCategory("network", lcli.NetCmd),
	lcli.WithCategory("network", lcli.SyncCmd),
	lcli.WithCategory("network", lcli.F3Cmd),
	lcli.WithCategory("status", lcli.StatusCmd),
	lcli.PprofCmd,
	lcli.VersionCmd,
}
