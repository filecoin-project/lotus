package tsresolver

import (
	"context"
	"errors"

	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/manifest"
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
	GetManifest(ctx context.Context) (*manifest.Manifest, error)
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
		return nil, xerrors.Errorf(`block param "%s" is not supported`, EthBlockSelectorEarliest)

	case EthBlockSelectorPending:
		return tsr.loader.GetHeaviestTipSet(), nil

	case EthBlockSelectorLatest:
		return tsr.loader.LoadTipSet(ctx, tsr.loader.GetHeaviestTipSet().Parents())

	case EthBlockSelectorSafe:
		return tsr.loadFinalizedTipSet(ctx, ethtypes.SafeEpochDelay)

	case EthBlockSelectorFinalized:
		return tsr.loadFinalizedTipSet(ctx, policy.ChainFinality)

	default:
		// likely an 0x hex block number or a decimal block number
		return tsr.resolveEthBlockNumberSelector(ctx, selector, strict)
	}
}

// loadFinalizedTipSet will use F3 to load and return the latest finalized tipset.
//
//   - If it can't load a finalized tipset from F3, it will return the tipset fallbackDelay epochs
//     behind the chain head (-1).
//   - If the finalized tipset from F3 is older than fallbackDelay epochs, it will return the tipset
//     fallbackDelay epochs behind the chain head (-1).
func (tsr *tipSetResolver) loadFinalizedTipSet(ctx context.Context, fallbackDelay abi.ChainEpoch) (*types.TipSet, error) {
	head := tsr.loader.GetHeaviestTipSet()
	latestHeight := head.Height() - 1 // always one behind for Eth compatibility due to deferred execution
	fallbackHeight := latestHeight - fallbackDelay

	f3TipSet, err := tsr.maybeF3FinalizedTipSet(ctx)
	if err != nil {
		return nil, err
	}

	if f3TipSet != nil && f3TipSet.Height() > fallbackHeight {
		// return the parent of the finalized tipset, which will be >= fallbackHeight
		return tsr.loader.LoadTipSet(ctx, f3TipSet.Parents())
	} // else F3 is disabled, not running, or behind the default safe or finalized height

	ts, err := tsr.loader.GetTipsetByHeight(ctx, fallbackHeight, head, true)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset at height %v: %w", fallbackHeight, err)
	}
	return ts, nil
}

// maybeF3FinalizedTipSet returns the latest F3 finalized tipset if F3 is enabled and the latest
// certificate indicates that the tipset is finalized on top of the EC chain. If F3 is disabled or
// or the current manifest is not finalizing tipsets on top of the EC chain, or we can't load a
// certificate with a finalized tipset, it returns nil. This method will only return errors that
// arise from dealing with the chain store, not from F3.
func (tsr *tipSetResolver) maybeF3FinalizedTipSet(ctx context.Context) (*types.TipSet, error) {
	// TODO: switch the order of GetManifest and GetLatestCert the former won't panic on
	// a fresh F3 instance: https://github.com/filecoin-project/lotus/issues/12772
	cert, err := tsr.f3.GetLatestCert(ctx)
	if err != nil {
		if !errors.Is(err, api.ErrF3Disabled) {
			log.Warnf("loading latest F3 certificate: %s", err)
		}
		return nil, nil
	}
	if cert == nil {
		return nil, nil
	}

	if manifest, err := tsr.f3.GetManifest(ctx); err != nil {
		log.Warnf("loading F3 manifest: %s", err)
		return nil, nil
	} else if !manifest.EC.Finalize {
		// F3 is not finalizing tipsets on top of EC, ignore it
		return nil, nil
	}

	if len(cert.ECChain) == 0 || cert.ECChain.IsZero() {
		// defensive; finality certs are supposed to have a guarantee of at least one tipset
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
		return nil, xerrors.New("requested a future epoch (beyond 'latest')")
	}

	ts, err := tsr.loader.GetTipsetByHeight(ctx, abi.ChainEpoch(num), head, true)
	if err != nil {
		return nil, xerrors.Errorf("cannot get tipset at height: %v", num)
	}

	if strict && ts.Height() != abi.ChainEpoch(num) {
		return nil, api.NewErrNullRound(abi.ChainEpoch(num))
	}

	return ts, nil
}
