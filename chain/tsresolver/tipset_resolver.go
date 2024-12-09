package tsresolver

import (
	"context"
	"errors"
	"fmt"

	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

var log = logging.Logger("chain/tsresolver")

const (
	EthBlockSelectorEarliest  = "earliest"
	EthBlockSelectorPending   = "pending"
	EthBlockSelectorLatest    = "latest"
	EthBlockSelectorSafe      = "safe"
	EthBlockSelectorFinalized = "finalized"
)

type TipSetLoader interface {
	GetHeaviestTipSet() (ts *types.TipSet)
	LoadTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
	GetTipsetByHeight(ctx context.Context, h abi.ChainEpoch, anchor *types.TipSet, prev bool) (*types.TipSet, error)
}

type F3 interface {
	GetLatestCert(ctx context.Context) (*certs.FinalityCertificate, error)
}

type TipSetResolver interface {
	ResolveEthBlockSelector(ctx context.Context, selector string, strict bool) (*types.TipSet, error)
}

type tipSetResolver struct {
	loader TipSetLoader
	f3     F3
}

var _ TipSetResolver = (*tipSetResolver)(nil)

func NewTipSetResolver(loader TipSetLoader, f3 F3) TipSetResolver {
	return &tipSetResolver{
		loader: loader,
		f3:     f3,
	}
}

// ResolveEthBlockSelector resolves an Ethereum block selector string to a TipSet.
//
// The selector can be one of:
//   - "pending": the chain head
//   - "latest": the TipSet with the latest executed messages (head - 1)
//   - "safe": the TipSet with messages executed at least eth.SafeEpochDelay (30) epochs ago or the
//     latest F3 finalized TipSet
//   - "finalized": the TipSet with messages executed at least policy.ChainFinality (900) epochs ago
//     or the latest F3 finalized TipSet
//   - a decimal block number: the TipSet at the given height
//   - a 0x-prefixed hex block number: the TipSet at the given height
//
// If a specific block number is specified and `strict` is true, an error is returned if the block
// number resolves to a null round, otherwise in the case of a null round the first non-null TipSet
// immediately before the null round is returned.
func (tsr *tipSetResolver) ResolveEthBlockSelector(ctx context.Context, selector string, strict bool) (*types.TipSet, error) {
	switch selector {
	case EthBlockSelectorEarliest:
		return nil, fmt.Errorf(`block param "%s" is not supported`, EthBlockSelectorEarliest)

	case EthBlockSelectorPending:
		return tsr.loader.GetHeaviestTipSet(), nil

	case EthBlockSelectorLatest:
		// head - 1 because we're always one behind for Eth compatibility due to deferred execution
		return tsr.loader.LoadTipSet(ctx, tsr.loader.GetHeaviestTipSet().Parents())

	case EthBlockSelectorSafe, EthBlockSelectorFinalized:
		defaultDelay := policy.ChainFinality
		if selector == EthBlockSelectorSafe {
			defaultDelay = ethtypes.SafeEpochDelay
		}

		head := tsr.loader.GetHeaviestTipSet()
		latestHeight := head.Height() - 1 // always one behind for Eth compatibility due to deferred execution
		defaultHeight := latestHeight - defaultDelay

		if f3TipSet, err := tsr.getF3FinalizedTipSet(ctx); err != nil {
			return nil, err
		} else if f3TipSet != nil && f3TipSet.Height() > defaultHeight {
			// return the parent of the finalized tipset (deferred execution, eth cares about t)
			return tsr.loader.LoadTipSet(ctx, f3TipSet.Parents())
		} // else F3 is disabled, not running, or behind the default safe or finalized height

		ts, err := tsr.loader.GetTipsetByHeight(ctx, defaultHeight, head, true)
		if err != nil {
			return nil, xerrors.Errorf("loading tipset at height %v: %w", defaultHeight, err)
		}
		return ts, nil

	default:
		// likely an 0x hex block number or a decimal block number
		return tsr.resolveEthBlockNumberSelector(ctx, selector, strict)
	}
}

func (tsr *tipSetResolver) getF3FinalizedTipSet(ctx context.Context) (*types.TipSet, error) {
	cert, err := tsr.f3.GetLatestCert(ctx)
	if err != nil {
		if !errors.Is(err, api.ErrF3Disabled) {
			log.Debugf("loading latest F3 certificate: %s", err)
		}
		return nil, nil
	}

	if len(cert.ECChain) == 0 || cert.ECChain.IsZero() {
		// finality certs are supposed to have a guarantee of at least one tipset but we'll take a defensive stance
		return nil, nil
	}

	tsk, err := types.TipSetKeyFromBytes(cert.ECChain.Head().Key)
	if err != nil {
		return nil, xerrors.Errorf("decoding tipset key reported by F3: %w", err)
	}

	finalizedTipSet, err := tsr.loader.LoadTipSet(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset reported as finalized by F3 %s: %w", tsk, err)
	}

	return finalizedTipSet, nil
}

func (tsr *tipSetResolver) resolveEthBlockNumberSelector(ctx context.Context, selector string, strict bool) (*types.TipSet, error) {
	var num ethtypes.EthUint64
	if err := num.UnmarshalJSON([]byte(`"` + selector + `"`)); err != nil {
		return nil, xerrors.Errorf("cannot parse block number: %v", err)
	}

	head := tsr.loader.GetHeaviestTipSet()
	if abi.ChainEpoch(num) > head.Height()-1 {
		return nil, errors.New("requested a future epoch (beyond 'latest')")
	}

	ts, err := tsr.loader.GetTipsetByHeight(ctx, abi.ChainEpoch(num), head, true)
	if err != nil {
		return nil, fmt.Errorf("cannot get tipset at height: %v", num)
	}

	if strict && ts.Height() != abi.ChainEpoch(num) {
		return nil, api.NewErrNullRound(abi.ChainEpoch(num))
	}

	return ts, nil
}
