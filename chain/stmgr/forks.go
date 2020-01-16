package stmgr

import (
	"context"

	"github.com/filecoin-project/lotus/chain/types"

	amt "github.com/filecoin-project/go-amt-ipld/v2"
	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

func (sm *StateManager) handleStateForks(ctx context.Context, pstate cid.Cid, height, parentH uint64) (_ cid.Cid, err error) {
	for i := parentH; i < height; i++ {
		switch i {
		}
	}

	return pstate, nil
}

func amtFsck(s types.Storage, ss cid.Cid) (cid.Cid, error) {
	a, err := amt.LoadAMT(types.WrapStorage(s), ss)
	if err != nil {
		return cid.Undef, xerrors.Errorf("could not load AMT: %w", a)
	}

	b := amt.NewAMT(types.WrapStorage(s))

	err = a.ForEach(func(id uint64, data *cbg.Deferred) error {
		err := b.Set(id, data)
		if err != nil {
			return xerrors.Errorf("could not copy at idx (%d): %w", id, err)
		}
		return nil
	})

	if err != nil {
		return cid.Undef, xerrors.Errorf("could not copy: %w", err)
	}

	nss, err := b.Flush()
	if err != nil {
		return cid.Undef, xerrors.Errorf("could not flush: %w", err)
	}

	return nss, nil
}
