package storageadapter

import (
	"bytes"
	"context"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

type getCurrentDealInfoAPI interface {
	StateMarketStorageDeal(context.Context, abi.DealID, types.TipSetKey) (*api.MarketDeal, error)
	StateSearchMsg(context.Context, cid.Cid) (*api.MsgLookup, error)
}

// GetCurrentDealInfo gets current information on a deal, and corrects the deal ID as needed
func GetCurrentDealInfo(ctx context.Context, ts *types.TipSet, api getCurrentDealInfoAPI, dealID abi.DealID, publishCid *cid.Cid) (abi.DealID, *api.MarketDeal, error) {
	marketDeal, dealErr := api.StateMarketStorageDeal(ctx, dealID, ts.Key())
	if dealErr == nil {
		return dealID, marketDeal, nil
	}
	if publishCid == nil {
		return dealID, nil, dealErr
	}
	// attempt deal id correction
	lookup, err := api.StateSearchMsg(ctx, *publishCid)
	if err != nil {
		return dealID, nil, err
	}

	if lookup.Receipt.ExitCode != exitcode.Ok {
		return dealID, nil, xerrors.Errorf("looking for publish deal message %s: non-ok exit code: %s", *publishCid, lookup.Receipt.ExitCode)
	}

	var retval market.PublishStorageDealsReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(lookup.Receipt.Return)); err != nil {
		return dealID, nil, xerrors.Errorf("looking for publish deal message: unmarshaling message return: %w", err)
	}

	if len(retval.IDs) != 1 {
		// market currently only ever sends messages with 1 deal
		return dealID, nil, xerrors.Errorf("can't recover dealIDs from publish deal message with more than 1 deal")
	}

	if retval.IDs[0] == dealID {
		// DealID did not change, so we are stuck with the original lookup error
		return dealID, nil, dealErr
	}

	dealID = retval.IDs[0]
	marketDeal, err = api.StateMarketStorageDeal(ctx, dealID, ts.Key())

	return dealID, marketDeal, err
}
