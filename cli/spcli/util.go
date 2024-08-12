package spcli

import (
	"context"
	"strconv"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

type ActorAddressGetter func(cctx *cli.Context) (address address.Address, err error)

// GetMinerAllDeals returns all deals for a miner
func GetMinerAllDeals(ctx context.Context, full api.FullNode, maddr address.Address, key types.TipSetKey) (dealIDs []uint64, err error) {
	allDeals, err := full.StateMarketDeals(ctx, key)
	if err != nil {
		return nil, err
	}

	// filter the deals for the miner
	var dealId uint64
	for k, d := range allDeals {
		if d.Proposal.Provider != maddr {
			continue
		}

		dealId, err = strconv.ParseUint(k, 10, 64)
		if err != nil {
			return nil, xerrors.Errorf("Error parsing deal id: %w", err)
		}

		dealIDs = append(dealIDs, dealId)
	}

	return dealIDs, nil
}
