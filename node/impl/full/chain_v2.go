package full

import (
	"context"
	"errors"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-f3"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/lf3"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

type ChainModuleAPIv2 interface {
	ChainGetTipSet(context.Context, types.TipSetSelector) (*types.TipSet, error)
}

type ChainModuleV2 struct {
	Chain *store.ChainStore
	F3    lf3.F3Backend `optional:"true"`

	fx.In
}

var _ ChainModuleAPIv2 = (*ChainModuleV2)(nil)

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

func (cm *ChainModuleV2) getTipSetByTag(ctx context.Context, tag types.TipSetTag) (*types.TipSet, error) {
	if tag == types.TipSetTags.Latest {
		return cm.Chain.GetHeaviestTipSet(), nil
	}
	if tag != types.TipSetTags.Finalized {
		return nil, xerrors.Errorf("unknown tipset tag: %s", tag)
	}

	if cm.F3 == nil {
		// F3 is disabled; fall back to EC finality.
		return cm.getECFinalized(ctx)
	}

	cert, err := cm.F3.GetLatestCert(ctx)
	if err != nil {
		if errors.Is(err, f3.ErrF3NotRunning) || errors.Is(err, api.ErrF3NotReady) {
			// Only fall back to EC finality if F3 isn't running or not ready.
			log.Debugw("F3 not running or not ready, falling back to EC finality", "err", err)
			return cm.getECFinalized(ctx)
		}
		return nil, err
	}
	if cert == nil {
		// No latest certificate. Fall back to EC finality.
		return cm.getECFinalized(ctx)
	}

	// Extract the finalized tipeset from the certificate.
	tsk, err := types.TipSetKeyFromBytes(cert.ECChain.Head().Key)
	if err != nil {
		return nil, xerrors.Errorf("decoding latest f3 cert tipset key: %w", err)
	}
	ts, err := cm.Chain.LoadTipSet(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading latest f3 cert tipset %s: %w", tsk, err)
	}
	return ts, nil
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
	head := cm.Chain.GetHeaviestTipSet()
	finalizedHeight := head.Height() - policy.ChainFinality
	return cm.Chain.GetTipsetByHeight(ctx, finalizedHeight, head, true)
}
