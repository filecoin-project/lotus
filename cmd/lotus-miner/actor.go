package main

import (
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"

	lapi "github.com/filecoin-project/lotus/api"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cli/spcli"
)

var actorCmd = &cli.Command{
	Name:  "actor",
	Usage: "manipulate the miner actor",
	Subcommands: []*cli.Command{
		spcli.ActorSetAddrsCmd(LMActorGetter),
		spcli.ActorWithdrawCmd(LMActorGetter),
		spcli.ActorRepayDebtCmd(LMActorGetter),
		spcli.ActorSetPeeridCmd(LMActorGetter),
		spcli.ActorSetOwnerCmd(LMConfigOrActorGetter),
		spcli.ActorControlCmd(LMConfigOrActorGetter, actorControlList),
		spcli.ActorProposeChangeWorkerCmd(LMActorGetter),
		spcli.ActorConfirmChangeWorkerCmd(LMActorGetter),
		spcli.ActorCompactAllocatedCmd(LMActorGetter),
		spcli.ActorProposeChangeBeneficiaryCmd(LMActorGetter),
		spcli.ActorConfirmChangeBeneficiaryCmd(LMConfigOrActorGetter),
	},
}

func LMConfigOrActorGetter(cctx *cli.Context) (address.Address, error) {
	ctx := lcli.ReqContext(cctx)
	return getActorAddress(ctx, cctx)
}

func actorControlList(cctx *cli.Context, actor address.Address) (lapi.AddressConfig, error) {
	minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
	if err != nil {
		return lapi.AddressConfig{}, err
	}
	defer closer()

	ctx := lcli.ReqContext(cctx)
	return minerApi.ActorAddressConfig(ctx)
}
