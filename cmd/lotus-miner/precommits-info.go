package main

import (
	"fmt"
	"sort"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"

	minertypes "github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/specs-actors/v7/actors/util/adt"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var sectorPreCommitsCmd = &cli.Command{
	Name:  "precommits",
	Usage: "Print on-chain precommit info",
	Action: func(cctx *cli.Context) error {
		ctx := lcli.ReqContext(cctx)
		mapi, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		maddr, err := getActorAddress(ctx, cctx)
		if err != nil {
			return err
		}
		mact, err := mapi.StateGetActor(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}
		store := adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(mapi)))
		mst, err := miner.Load(store, mact)
		if err != nil {
			return err
		}
		preCommitSector := make([]minertypes.SectorPreCommitOnChainInfo, 0)
		err = mst.ForEachPrecommittedSector(func(info minertypes.SectorPreCommitOnChainInfo) error {
			preCommitSector = append(preCommitSector, info)
			return err
		})
		less := func(i, j int) bool {
			return preCommitSector[i].Info.SectorNumber <= preCommitSector[j].Info.SectorNumber
		}
		sort.Slice(preCommitSector, less)
		for _, info := range preCommitSector {
			fmt.Printf("%s: %s\n", info.Info.SectorNumber, info.PreCommitEpoch)
		}

		return nil
	},
}
