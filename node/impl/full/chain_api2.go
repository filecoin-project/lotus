package full

import (
	"context"
	"fmt"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v2api"
	"github.com/filecoin-project/lotus/chain/lf3"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type ChainModuleAPIv2 interface {
	ChainNotify(ctx context.Context, p jsonrpc.RawParams) (<-chan []*api.HeadChange, error)
	ChainHead(ctx context.Context, p jsonrpc.RawParams) (*types.TipSet, error)
	ChainGetMessagesInTipset(ctx context.Context, tss types.TipSetSelector) ([]api.Message, error)
	ChainGetTipSetByHeight(ctx context.Context, epoch abi.ChainEpoch, tss types.TipSetSelector) (*types.TipSet, error)
	ChainGetTipSetAfterHeight(ctx context.Context, epoch abi.ChainEpoch, tss types.TipSetSelector) (*types.TipSet, error)
}

var _ ChainModuleAPIv2 = *new(v2api.FullNode)

type ChainModulev2 struct {
	Chain             *store.ChainStore
	F3                *lf3.F3
	ExposedBlockstore dtypes.ExposedBlockstore

	fx.In
}

var _ ChainModuleAPIv2 = (*ChainModulev2)(nil)

func (cm *ChainModulev2) ChainNotify(ctx context.Context, p jsonrpc.RawParams) (<-chan []*api.HeadChange, error) {
	// TODO: consume param
	return cm.Chain.SubHeadChanges(ctx), nil
}

func (cm *ChainModulev2) ChainHead(ctx context.Context, p jsonrpc.RawParams) (*types.TipSet, error) {
	param, err := types.DecodeOptionalEpochDescriptorArg(p)
	if err != nil {
		return nil, xerrors.Errorf("decoding optional epoch descriptor parameter: %w", err)
	}

	switch param {
	case types.EpochLatest:
		return cm.Chain.GetHeaviestTipSet(), nil
	case types.EpochFinalized:
		if ts, err := lf3.ResolveEpochDescriptorTipSet(ctx, cm.F3, cm.Chain, param); err != nil {
			return nil, xerrors.Errorf("resolving epoch descriptor tipset: %w", err)
		} else {
			return ts, nil
		}
	default:
		return nil, fmt.Errorf("unsupported epoch descriptor: [%v]", param)
	}
}

func (cm *ChainModulev2) ChainGetMessagesInTipset(ctx context.Context, tss types.TipSetSelector) ([]api.Message, error) {
	ts, err := lf3.ResolveTipSetSelector(ctx, cm.F3, cm.Chain, tss)
	if err != nil {
		return nil, xerrors.Errorf("resolving tipset selector: %w", err)
	}

	if ts.Height() == 0 { // genesis block has no parent messages
		return nil, nil
	}

	cmes, err := cm.Chain.MessagesForTipset(ctx, ts)
	if err != nil {
		return nil, err
	}

	// when len(cmes) == 0, JSON serialization should return an empty array, not null
	out := make([]api.Message, len(cmes))
	for i, m := range cmes {
		out[i] = api.Message{
			Cid:     m.Cid(),
			Message: m.VMMessage(),
		}
	}

	return out, nil
}

func (cm *ChainModulev2) ChainGetTipSetByHeight(ctx context.Context, epoch abi.ChainEpoch, tss types.TipSetSelector) (*types.TipSet, error) {
	if ts, err := lf3.ResolveTipSetSelector(ctx, cm.F3, cm.Chain, tss); err != nil {
		return nil, xerrors.Errorf("resolving tipset selector: %w", err)
	} else {
		return cm.Chain.GetTipsetByHeight(ctx, epoch, ts, true)
	}
}

func (cm *ChainModulev2) ChainGetTipSetAfterHeight(ctx context.Context, epoch abi.ChainEpoch, tss types.TipSetSelector) (*types.TipSet, error) {
	if ts, err := lf3.ResolveTipSetSelector(ctx, cm.F3, cm.Chain, tss); err != nil {
		return nil, xerrors.Errorf("resolving tipset selector: %w", err)
	} else {
		return cm.Chain.GetTipsetByHeight(ctx, epoch, ts, false)
	}
}
