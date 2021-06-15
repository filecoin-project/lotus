package testkit

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"

	tstats "github.com/filecoin-project/lotus/tools/stats"
)

func StartDeal(ctx context.Context, minerActorAddr address.Address, client api.FullNode, fcid cid.Cid, fastRetrieval bool) *cid.Cid {
	addr, err := client.WalletDefaultAddress(ctx)
	if err != nil {
		panic(err)
	}

	deal, err := client.ClientStartDeal(ctx, &api.StartDealParams{
		Data: &storagemarket.DataRef{
			TransferType: storagemarket.TTGraphsync,
			Root:         fcid,
		},
		Wallet:            addr,
		Miner:             minerActorAddr,
		EpochPrice:        types.NewInt(4000000),
		MinBlocksDuration: 640000,
		DealStartEpoch:    200,
		FastRetrieval:     fastRetrieval,
	})
	if err != nil {
		panic(err)
	}
	return deal
}

func WaitDealSealed(t *TestEnvironment, ctx context.Context, client api.FullNode, deal *cid.Cid) {
	height := 0
	headlag := 3

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	tipsetsCh, err := tstats.GetTips(cctx, &v0api.WrapperV1Full{FullNode: client}, abi.ChainEpoch(height), headlag)
	if err != nil {
		panic(err)
	}

	for tipset := range tipsetsCh {
		t.RecordMessage("got tipset: height %d", tipset.Height())

		di, err := client.ClientGetDealInfo(ctx, *deal)
		if err != nil {
			panic(err)
		}
		switch di.State {
		case storagemarket.StorageDealProposalRejected:
			panic("deal rejected")
		case storagemarket.StorageDealFailing:
			panic("deal failed")
		case storagemarket.StorageDealError:
			panic(fmt.Sprintf("deal errored %s", di.Message))
		case storagemarket.StorageDealActive:
			t.RecordMessage("completed deal: %s", di)
			return
		}

		t.RecordMessage("deal state: %s", storagemarket.DealStates[di.State])
	}
}
