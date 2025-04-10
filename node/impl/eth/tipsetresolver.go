package eth

import (
	"context"
	"errors"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-f3"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/lf3"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

var _ TipSetResolver = (*tipSetResolver)(nil)

type tipSetResolver struct {
	cs ChainStore
	f3 lf3.F3Backend // can be nil if disabled or this is a v1 resolver
}

func NewV1TipSetResolver(cs ChainStore) TipSetResolver {
	return &tipSetResolver{cs: cs, f3: nil}
}

func NewV2TipSetResolver(cs ChainStore, f3 lf3.F3Backend) TipSetResolver {
	return &tipSetResolver{cs: cs, f3: f3}
}

func (tsr *tipSetResolver) getTipSetByTag(ctx context.Context, tag string) (*types.TipSet, error) {
	if tsr.f3 == nil {
		// F3 is disabled; fall back to EC finality.
		return tsr.getECTipSetByTag(ctx, tag)
	}

	cert, err := tsr.f3.GetLatestCert(ctx)
	if err != nil {
		if errors.Is(err, f3.ErrF3NotRunning) || errors.Is(err, api.ErrF3NotReady) {
			// Only fall back to EC finality if F3 isn't running or not ready.
			log.Debugw("F3 not running or not ready, falling back to EC finality", "err", err)
			return tsr.getECTipSetByTag(ctx, tag)
		}
		return nil, err
	}
	if cert == nil {
		// No latest certificate. Fall back to EC finality.
		return tsr.getECTipSetByTag(ctx, tag)
	}

	// Extract the finalized tipeset from the certificate.
	tsk, err := types.TipSetKeyFromBytes(cert.ECChain.Head().Key)
	if err != nil {
		return nil, xerrors.Errorf("decoding latest f3 cert tipset key: %w", err)
	}
	ts, err := tsr.cs.LoadTipSet(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading latest f3 cert tipset %s: %w", tsk, err)
	}
	return ts, nil
}

func (tsr *tipSetResolver) getECTipSetByTag(ctx context.Context, tag string) (*types.TipSet, error) {
	head := tsr.cs.GetHeaviestTipSet()
	// for Ethereum APIs we focus on the message inclusion tipset so we have to -1 here, in contrast
	// to the Filecoin native height selectors
	height := head.Height() - 1 - policy.ChainFinality
	if tag == "safe" {
		height = head.Height() - 1 - ethtypes.SafeEpochDelay
	}
	return tsr.cs.GetTipsetByHeight(ctx, height, head, true)
}

func (tsr *tipSetResolver) GetTipSetByHash(ctx context.Context, blkParam ethtypes.EthHash) (*types.TipSet, error) {
	ts, err := tsr.cs.GetTipSetByCid(ctx, blkParam.ToCid())
	if err != nil {
		return nil, xerrors.Errorf("cannot get tipset by hash: %v", err)
	}
	if ts == nil {
		return nil, xerrors.Errorf("cannot find tipset for block hash %s", blkParam)
	}
	return ts, nil
}

func (tsr *tipSetResolver) GetTipsetByBlockNumber(ctx context.Context, blkParam string, strict bool) (*types.TipSet, error) {
	if blkParam == "earliest" {
		return nil, xerrors.New("block param \"earliest\" is not supported")
	}

	head := tsr.cs.GetHeaviestTipSet()
	switch blkParam {
	case "pending":
		return head, nil
	case "latest":
		parent, err := tsr.cs.GetTipSetFromKey(ctx, head.Parents())
		if err != nil {
			return nil, xerrors.New("cannot get parent tipset")
		}
		return parent, nil
	case "safe":
		if tsr.f3 != nil {
			return tsr.getTipSetByTag(ctx, "safe")
		}
		return tsr.getECTipSetByTag(ctx, "safe")
	case "finalized":
		if tsr.f3 != nil {
			return tsr.getTipSetByTag(ctx, "finalized")
		}
		return tsr.getECTipSetByTag(ctx, "finalized")
	default:
		var num ethtypes.EthUint64
		err := num.UnmarshalJSON([]byte(`"` + blkParam + `"`))
		if err != nil {
			return nil, xerrors.Errorf("cannot parse block number: %v", err)
		}
		if abi.ChainEpoch(num) > head.Height()-1 {
			return nil, xerrors.New("requested a future epoch (beyond 'latest')")
		}
		ts, err := tsr.cs.GetTipsetByHeight(ctx, abi.ChainEpoch(num), head, true)
		if err != nil {
			return nil, xerrors.Errorf("cannot get tipset at height: %v", num)
		}
		if strict && ts.Height() != abi.ChainEpoch(num) {
			return nil, api.NewErrNullRound(abi.ChainEpoch(num))
		}
		return ts, nil
	}
}

func (tsr *tipSetResolver) GetTipsetByBlockNumberOrHash(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash) (*types.TipSet, error) {
	head := tsr.cs.GetHeaviestTipSet()

	predefined := blkParam.PredefinedBlock
	if predefined != nil {
		if *predefined == "earliest" {
			return nil, xerrors.New("block param \"earliest\" is not supported")
		} else if *predefined == "pending" {
			return head, nil
		} else if *predefined == "latest" {
			parent, err := tsr.cs.GetTipSetFromKey(ctx, head.Parents())
			if err != nil {
				return nil, xerrors.New("cannot get parent tipset")
			}
			return parent, nil
		}
		// TODO: we support "safe" and "finalized" here
		return nil, xerrors.Errorf("unknown predefined block %s", *predefined)
	}

	if blkParam.BlockNumber != nil {
		height := abi.ChainEpoch(*blkParam.BlockNumber)
		if height > head.Height()-1 {
			return nil, xerrors.New("requested a future epoch (beyond 'latest')")
		}
		ts, err := tsr.cs.GetTipsetByHeight(ctx, height, head, true)
		if err != nil {
			return nil, xerrors.Errorf("cannot get tipset at height: %v", height)
		}
		return ts, nil
	}

	if blkParam.BlockHash != nil {
		ts, err := tsr.cs.GetTipSetByCid(ctx, blkParam.BlockHash.ToCid())
		if err != nil {
			return nil, xerrors.Errorf("cannot get tipset by hash: %v", err)
		}

		// verify that the tipset is in the canonical chain
		if blkParam.RequireCanonical {
			// walk up the current chain (our head) until we reach ts.Height()
			walkTs, err := tsr.cs.GetTipsetByHeight(ctx, ts.Height(), head, true)
			if err != nil {
				return nil, xerrors.Errorf("cannot get tipset at height: %v", ts.Height())
			}

			// verify that it equals the expected tipset
			if !walkTs.Equals(ts) {
				return nil, xerrors.New("tipset is not canonical")
			}
		}

		return ts, nil
	}

	return nil, xerrors.New("invalid block param")
}
