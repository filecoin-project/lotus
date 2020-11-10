package storageadapter

import (
	"bytes"
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

type getCurrentDealInfoAPI interface {
	StateLookupID(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateMarketStorageDeal(context.Context, abi.DealID, types.TipSetKey) (*api.MarketDeal, error)
	StateSearchMsg(context.Context, cid.Cid) (*api.MsgLookup, error)
}

// GetCurrentDealInfo gets current information on a deal, and corrects the deal ID as needed
func GetCurrentDealInfo(ctx context.Context, ts *types.TipSet, api getCurrentDealInfoAPI, dealID abi.DealID, proposal market.DealProposal, publishCid *cid.Cid) (abi.DealID, *api.MarketDeal, error) {
	marketDeal, dealErr := api.StateMarketStorageDeal(ctx, dealID, ts.Key())
	if dealErr == nil {
		equal, err := checkDealEquality(ctx, ts, api, proposal, marketDeal.Proposal)
		if err != nil {
			return dealID, nil, err
		}
		if equal {
			return dealID, marketDeal, nil
		}
		dealErr = xerrors.Errorf("Deal proposals did not match")
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

	if err == nil {
		equal, err := checkDealEquality(ctx, ts, api, proposal, marketDeal.Proposal)
		if err != nil {
			return dealID, nil, err
		}
		if !equal {
			return dealID, nil, xerrors.Errorf("Deal proposals did not match")
		}
	}
	return dealID, marketDeal, err
}

func checkDealEquality(ctx context.Context, ts *types.TipSet, api getCurrentDealInfoAPI, p1, p2 market.DealProposal) (bool, error) {
	p1ClientID, err := api.StateLookupID(ctx, p1.Client, ts.Key())
	if err != nil {
		return false, err
	}
	p2ClientID, err := api.StateLookupID(ctx, p2.Client, ts.Key())
	if err != nil {
		return false, err
	}
	return p1.PieceCID.Equals(p2.PieceCID) &&
		p1.PieceSize == p2.PieceSize &&
		p1.VerifiedDeal == p2.VerifiedDeal &&
		p1.Label == p2.Label &&
		p1.StartEpoch == p2.StartEpoch &&
		p1.EndEpoch == p2.EndEpoch &&
		p1.StoragePricePerEpoch.Equals(p2.StoragePricePerEpoch) &&
		p1.ProviderCollateral.Equals(p2.ProviderCollateral) &&
		p1.ClientCollateral.Equals(p2.ClientCollateral) &&
		p1.Provider == p2.Provider &&
		p1ClientID == p2ClientID, nil
}
