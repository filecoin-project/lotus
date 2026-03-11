package full

import (
	"context"
	"errors"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-f3"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/ecfinality"
	"github.com/filecoin-project/lotus/chain/lf3"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ ChainModuleAPIv2 = (*ChainModuleV2)(nil)

type ChainModuleAPIv2 interface {
	ChainGetTipSet(context.Context, types.TipSetSelector) (*types.TipSet, error)
	ChainGetTipSetFinalityStatus(context.Context) (*types.FinalityStatus, error)
}

type ChainModuleV2 struct {
	Chain      *store.ChainStore
	F3         lf3.F3Backend       `optional:"true"`
	ECFinality ecfinality.Provider `optional:"true"`

	fx.In
}

func (cm *ChainModuleV2) ChainGetTipSet(ctx context.Context, selector types.TipSetSelector) (*types.TipSet, error) {
	if err := selector.Validate(); err != nil {
		return nil, xerrors.Errorf("validating selector: %w", err)
	}

	// Get tipset by key.
	if selector.Key != nil {
		return cm.Chain.GetTipSetFromKey(ctx, *selector.Key)
	}

	// Get tipset by height.
	if selector.Height != nil {
		anchor, err := cm.getTipSetByAnchor(ctx, selector.Height.Anchor)
		if err != nil {
			return nil, xerrors.Errorf("getting anchor from tipset: %w", err)
		}
		return cm.Chain.GetTipsetByHeight(ctx, *selector.Height.At, anchor, selector.Height.Previous)
	}

	// Get tipset by tag, either latest or finalized.
	if selector.Tag != nil {
		return cm.getTipSetByTag(ctx, *selector.Tag)
	}

	return nil, xerrors.Errorf("no tipset found for selector")
}

func (cm *ChainModuleV2) ChainGetTipSetFinalityStatus(ctx context.Context) (*types.FinalityStatus, error) {
	head := cm.Chain.GetHeaviestTipSet()
	if head == nil {
		return nil, xerrors.New("no known heaviest tipset")
	}

	status := &types.FinalityStatus{
		Head:                     head,
		ECFinalityThresholdDepth: -1,
	}

	// EC calculator result
	if cm.ECFinality != nil {
		ecStatus, err := cm.ECFinality.GetStatus(ctx)
		if err != nil {
			log.Debugw("EC finality calculator error in status query", "err", err)
		} else {
			status.ECFinalityThresholdDepth = ecStatus.ThresholdDepth
			status.ECFinalizedTipSet = ecStatus.FinalizedTipSet
		}
	}

	// EC finalized tipset: use the calculator result if available, otherwise
	// fall back to static head - policy.ChainFinality.
	ecFinalized := status.ECFinalizedTipSet
	if ecFinalized == nil {
		finalizedHeight := max(0, head.Height()-policy.ChainFinality)
		ts, err := cm.Chain.GetTipsetByHeight(ctx, finalizedHeight, head, true)
		if err != nil {
			return nil, xerrors.Errorf("getting static EC finalized tipset: %w", err)
		}
		ecFinalized = ts
	}
	status.FinalizedTipSet = ecFinalized

	// F3 result: if F3 is ahead of EC, use that as the overall finalized tipset.
	if cm.F3 != nil {
		cert, err := cm.F3.GetLatestCert(ctx)
		if err != nil {
			if !errors.Is(err, f3.ErrF3NotRunning) && !errors.Is(err, api.ErrF3NotReady) {
				return nil, xerrors.Errorf("getting F3 certificate: %w", err)
			}
		} else if cert != nil {
			f3Epoch := abi.ChainEpoch(cert.ECChain.Head().Epoch)
			// Only use F3 if it's within a plausible range of EC finality.
			if head.Height()-f3Epoch <= policy.ChainFinality {
				tsk, err := types.TipSetKeyFromBytes(cert.ECChain.Head().Key)
				if err != nil {
					return nil, xerrors.Errorf("decoding F3 cert tipset key: %w", err)
				}
				f3Ts, err := cm.Chain.LoadTipSet(ctx, tsk)
				if err != nil {
					return nil, xerrors.Errorf("loading F3 cert tipset %s: %w", tsk, err)
				}
				status.F3FinalizedTipSet = f3Ts
				if f3Ts.Height() > status.FinalizedTipSet.Height() {
					status.FinalizedTipSet = f3Ts
				}
			}
		}
	}

	return status, nil
}

func (cm *ChainModuleV2) getTipSetByTag(ctx context.Context, tag types.TipSetTag) (*types.TipSet, error) {
	switch tag {
	case types.TipSetTags.Latest:
		return cm.Chain.GetHeaviestTipSet(), nil
	case types.TipSetTags.Finalized:
		return cm.getLatestFinalizedTipset(ctx)
	case types.TipSetTags.Safe:
		return cm.getLatestSafeTipSet(ctx)
	default:
		return nil, xerrors.Errorf("unknown tipset tag: %s", tag)
	}
}

func (cm *ChainModuleV2) getLatestSafeTipSet(ctx context.Context) (*types.TipSet, error) {
	finalized, err := cm.getLatestFinalizedTipset(ctx)
	if err != nil {
		return nil, xerrors.Errorf("getting latest finalized tipset: %w", err)
	}
	heaviest := cm.Chain.GetHeaviestTipSet()
	if heaviest == nil {
		return nil, xerrors.Errorf("no known heaviest tipset")
	}
	safeHeight := max(0, heaviest.Height()-buildconstants.SafeHeightDistance)
	if finalized != nil && finalized.Height() >= safeHeight {
		return finalized, nil
	}
	return cm.Chain.GetTipsetByHeight(ctx, safeHeight, heaviest, true)
}

func (cm *ChainModuleV2) getLatestFinalizedTipset(ctx context.Context) (*types.TipSet, error) {
	// EC finality: use the FRC-0089 calculator when available, otherwise static
	// head-900 fallback. This is always computed so it can be compared with F3.
	ecTs, err := cm.getECFinalized(ctx)
	if err != nil {
		return nil, err
	}

	if cm.F3 == nil {
		// F3 is disabled; fall back to EC finality.
		return ecTs, nil
	}

	cert, err := cm.F3.GetLatestCert(ctx)
	if err != nil {
		if errors.Is(err, f3.ErrF3NotRunning) || errors.Is(err, api.ErrF3NotReady) {
			log.Debugw("F3 not running or not ready, using EC finality", "err", err)
			return ecTs, nil
		}
		return nil, err
	}

	// If not operating, or the F3 finalized tipset is behind EC finality, just use EC finality.
	if cert == nil || abi.ChainEpoch(cert.ECChain.Head().Epoch) <= ecTs.Height() {
		return ecTs, nil
	}

	tsk, err := types.TipSetKeyFromBytes(cert.ECChain.Head().Key)
	if err != nil {
		return nil, xerrors.Errorf("decoding latest f3 cert tipset key: %w", err)
	}
	f3Ts, err := cm.Chain.LoadTipSet(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading latest f3 cert tipset %s: %w", tsk, err)
	}
	return f3Ts, nil
}

func (cm *ChainModuleV2) getTipSetByAnchor(ctx context.Context, anchor *types.TipSetAnchor) (*types.TipSet, error) {
	switch {
	case anchor == nil:
		// No anchor specified. Fall back to finalized tipset.
		return cm.getTipSetByTag(ctx, types.TipSetTags.Finalized)
	case anchor.Key == nil && anchor.Tag == nil:
		// Anchor is zero-valued. Fall back to heaviest tipset.
		return cm.Chain.GetHeaviestTipSet(), nil
	case anchor.Key != nil:
		// Get tipset at the specified key.
		return cm.Chain.GetTipSetFromKey(ctx, *anchor.Key)
	case anchor.Tag != nil:
		return cm.getTipSetByTag(ctx, *anchor.Tag)
	default:
		return nil, xerrors.Errorf("invalid anchor: %v", anchor)
	}
}

func (cm *ChainModuleV2) getECFinalized(ctx context.Context) (*types.TipSet, error) {
	// Use the FRC-0089 calculator for probabilistic EC finality when available.
	if cm.ECFinality != nil {
		ts, err := cm.ECFinality.GetFinalizedTipSet(ctx)
		if err != nil {
			log.Debugw("EC finality calculator error, falling back to static finality", "err", err)
		} else if ts != nil {
			return ts, nil
		}
		// Calculator returned nil (threshold not met), fall through to static.
	}
	head := cm.Chain.GetHeaviestTipSet()
	if head == nil {
		return nil, xerrors.New("no known heaviest tipset")
	}
	finalizedHeight := max(0, head.Height()-policy.ChainFinality)
	return cm.Chain.GetTipsetByHeight(ctx, finalizedHeight, head, true)
}
