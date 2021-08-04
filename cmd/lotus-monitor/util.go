package main

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
)

// It is necessary to convert attofil to fil before
// emitting it as a metric to avoid overflow. There are
// no uint64 openmetrics measurements.
func filBalance(attofil types.BigInt) float64 {
	return types.BigDivFloat(attofil, types.FromFil(1))
}

func verifiedPower(cctx *cli.Context, api v0api.FullNode, addr address.Address) (bool, abi.StoragePower, error) {
	ctx := cctx.Context
	id, err := api.StateLookupID(ctx, addr, types.EmptyTSK)
	if err != nil {
		return false, big.Zero(), err
	}

	actor, err := api.StateGetActor(ctx, verifreg.Address, types.EmptyTSK)
	if err != nil {
		return false, big.Zero(), err
	}

	apibs := blockstore.NewAPIBlockstore(api)
	store := adt.WrapStore(ctx, cbor.NewCborStore(apibs))

	s, err := verifreg.Load(store, actor)
	if err != nil {
		return false, big.Zero(), err
	}

	return s.VerifierDataCap(id)
}
