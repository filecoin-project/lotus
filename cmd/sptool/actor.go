package main

import (
	"os"
	"path"

	"github.com/BurntSushi/toml"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/cli/spcli"
)

var actorCmd = &cli.Command{
	Name:  "actor",
	Usage: "Manage Filecoin Miner Actor Metadata",
	Subcommands: []*cli.Command{
		spcli.ActorSetAddrsCmd(SPTActorGetter),
		spcli.ActorWithdrawCmd(SPTActorGetter),
		spcli.ActorRepayDebtCmd(SPTActorGetter),
		spcli.ActorSetPeeridCmd(SPTActorGetter),
		spcli.ActorSetOwnerCmd(SPTActorGetter),
		addTomlArg(spcli.ActorControlCmd(SPTActorGetter, getControlAddresses)),
		spcli.ActorProposeChangeWorkerCmd(SPTActorGetter),
		spcli.ActorConfirmChangeWorkerCmd(SPTActorGetter),
		spcli.ActorCompactAllocatedCmd(SPTActorGetter),
		spcli.ActorProposeChangeBeneficiaryCmd(SPTActorGetter),
		spcli.ActorConfirmChangeBeneficiaryCmd(SPTActorGetter),
	},
}

func addTomlArg(cmd *cli.Command) *cli.Command {
	cmd.Flags = append(cmd.Flags, &cli.StringFlag{
		Name:        "toml",
		Usage:       "path to toml file, or folder containing config.toml",
		EnvVars:     []string{"LOTUS_MINER_PATH"},
		DefaultText: "~/.lotusminer",
	})
	return cmd
}

func getControlAddresses(cctx *cli.Context, actor address.Address) (lapi.AddressConfig, error) {
	p, err := homedir.Expand(cctx.String("toml"))
	if err != nil {
		return lapi.AddressConfig{}, err
	}
	st, err := os.Stat(p)
	if err != nil {
		return lapi.AddressConfig{}, err
	}
	if st.IsDir() {
		p = path.Join(p, "config.toml")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return lapi.AddressConfig{}, err
	}
	var cfg struct {
		Addresses lapi.AddressConfig
	}
	_, err = toml.Decode(string(b), &cfg)
	if err != nil {
		return lapi.AddressConfig{}, err
	}
	return cfg.Addresses, nil
}
