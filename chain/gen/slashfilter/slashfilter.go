package slashfilter

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
)

type SlashFilter struct {
	byEpoch   ds.Datastore // double-fork mining faults, parent-grinding fault
	byParents ds.Datastore // time-offset mining faults
}

func New(dstore ds.Batching) *SlashFilter {
	return &SlashFilter{
		byEpoch:   namespace.Wrap(dstore, ds.NewKey("/slashfilter/epoch")),
		byParents: namespace.Wrap(dstore, ds.NewKey("/slashfilter/parents")),
	}
}

func (f *SlashFilter) MinedBlock(ctx context.Context, bh *types.BlockHeader, parentEpoch abi.ChainEpoch) (cid.Cid, error) {
	epochKey := ds.NewKey(fmt.Sprintf("/%s/%d", bh.Miner, bh.Height))
	{
		// double-fork mining (2 blocks at one epoch)
		if witness, err := checkFault(ctx, f.byEpoch, epochKey, bh, "double-fork mining faults"); err != nil {
			return witness, xerrors.Errorf("check double-fork mining faults: %w", err)
		}
	}

	parentsKey := ds.NewKey(fmt.Sprintf("/%s/%x", bh.Miner, types.NewTipSetKey(bh.Parents...).Bytes()))
	{
		// time-offset mining faults (2 blocks with the same parents)
		if witness, err := checkFault(ctx, f.byParents, parentsKey, bh, "time-offset mining faults"); err != nil {
			return witness, xerrors.Errorf("check time-offset mining faults: %w", err)
		}
	}

	{
		// parent-grinding fault (didn't mine on top of our own block)

		// First check if we have mined a block on the parent epoch
		parentEpochKey := ds.NewKey(fmt.Sprintf("/%s/%d", bh.Miner, parentEpoch))
		have, err := f.byEpoch.Has(ctx, parentEpochKey)
		if err != nil {
			return cid.Undef, err
		}

		if have {
			// If we had, make sure it's in our parent tipset
			cidb, err := f.byEpoch.Get(ctx, parentEpochKey)
			if err != nil {
				return cid.Undef, xerrors.Errorf("getting other block cid: %w", err)
			}

			_, parent, err := cid.CidFromBytes(cidb)
			if err != nil {
				return cid.Undef, err
			}

			var found bool
			for _, c := range bh.Parents {
				if c.Equals(parent) {
					found = true
				}
			}

			if !found {
				return parent, xerrors.Errorf("produced block would trigger 'parent-grinding fault' consensus fault; miner: %s; bh: %s, expected parent: %s", bh.Miner, bh.Cid(), parent)
			}
		}
	}

	if err := f.byParents.Put(ctx, parentsKey, bh.Cid().Bytes()); err != nil {
		return cid.Undef, xerrors.Errorf("putting byEpoch entry: %w", err)
	}

	if err := f.byEpoch.Put(ctx, epochKey, bh.Cid().Bytes()); err != nil {
		return cid.Undef, xerrors.Errorf("putting byEpoch entry: %w", err)
	}

	return cid.Undef, nil
}

func checkFault(ctx context.Context, t ds.Datastore, key ds.Key, bh *types.BlockHeader, faultType string) (cid.Cid, error) {
	fault, err := t.Has(ctx, key)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to read from datastore: %w", err)
	}

	if fault {
		cidb, err := t.Get(ctx, key)
		if err != nil {
			return cid.Undef, xerrors.Errorf("getting other block cid: %w", err)
		}

		_, other, err := cid.CidFromBytes(cidb)
		if err != nil {
			return cid.Undef, err
		}

		if other == bh.Cid() {
			return cid.Undef, nil
		}

		return other, xerrors.Errorf("produced block would trigger '%s' consensus fault; miner: %s; bh: %s, other: %s", faultType, bh.Miner, bh.Cid(), other)
	}

	return cid.Undef, nil
}
