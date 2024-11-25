package full

import (
	"context"
	"errors"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-f3"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v2api"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/lf3"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

type ChainModuleAPIv2 interface {
	v2api.Chain
}

type ChainModuleV2 struct {
	Chain *store.ChainStore
	F3    lf3.F3Backend

	fx.In
}

var _ ChainModuleAPIv2 = (*ChainModuleV2)(nil)

func (cm *ChainModuleV2) ChainGetTipSet(ctx context.Context, selector types.TipSetSelector) (*types.TipSet, error) {
	criterion, err := types.DecodeTipSetCriterion(selector)
	if err != nil {
		return nil, xerrors.Errorf("getting selector criterion: %w", err)
	}
	if criterion == nil {
		// Fall back to default selector, latest, if no selector is provided.
		criterion = &types.TipSetCriterion{
			Tag: &types.TipSetTags.Latest,
		}
	}

	// Get tipset by key.
	if criterion.Key != nil {
		return cm.Chain.GetTipSetFromKey(ctx, *criterion.Key)
	}

	// Get tipset by height.
	if criterion.Height != nil {
		head := cm.Chain.GetHeaviestTipSet()
		return cm.Chain.GetTipsetByHeight(ctx, criterion.Height.At, head, criterion.Height.Previous)
	}

	// Get tipset by tag, either latest or finalized.
	if criterion.Tag != nil {
		return cm.getTipSetByTag(ctx, *criterion.Tag)
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

func (cm *ChainModuleV2) getECFinalized(ctx context.Context) (*types.TipSet, error) {
	head := cm.Chain.GetHeaviestTipSet()
	finalizedHeight := head.Height() - policy.ChainFinality
	return cm.Chain.GetTipsetByHeight(ctx, finalizedHeight, head, true)
}
