package storageadapter

import (
	"context"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors/adt"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
)

type apiWrapper struct {
	api interface {
		StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error)
		ChainReadObj(context.Context, cid.Cid) ([]byte, error)
		ChainHasObj(context.Context, cid.Cid) (bool, error)
	}
}

func (ca *apiWrapper) diffPreCommits(ctx context.Context, actor address.Address, pre, cur types.TipSetKey) (*miner.PreCommitChanges, error) {
	store := adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(ca.api)))

	preAct, err := ca.api.StateGetActor(ctx, actor, pre)
	if err != nil {
		return nil, xerrors.Errorf("getting pre actor: %w", err)
	}
	curAct, err := ca.api.StateGetActor(ctx, actor, cur)
	if err != nil {
		return nil, xerrors.Errorf("getting cur actor: %w", err)
	}

	preSt, err := miner.Load(store, preAct)
	if err != nil {
		return nil, xerrors.Errorf("loading miner actor: %w", err)
	}
	curSt, err := miner.Load(store, curAct)
	if err != nil {
		return nil, xerrors.Errorf("loading miner actor: %w", err)
	}

	diff, err := miner.DiffPreCommits(preSt, curSt)
	if err != nil {
		return nil, xerrors.Errorf("diff precommits: %w", err)
	}

	return diff, err
}
