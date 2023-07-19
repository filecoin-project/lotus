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

func (f *SlashFilter) MinedBlock(ctx context.Context, bh *types.BlockHeader, parentEpoch abi.ChainEpoch) (cid.Cid, bool, error) {
	epochKey := ds.NewKey(fmt.Sprintf("/%s/%d", bh.Miner, bh.Height))
	{
		// double-fork mining (2 blocks at one epoch)
		doubleForkWitness, doubleForkFault, err := checkFault(ctx, f.byEpoch, epochKey, bh, "double-fork mining faults")
		if err != nil {
			return cid.Undef, false, xerrors.Errorf("check double-fork mining faults: %w", err)
		}

		if doubleForkFault {
			return doubleForkWitness, doubleForkFault, nil
		}
	}

	parentsKey := ds.NewKey(fmt.Sprintf("/%s/%x", bh.Miner, types.NewTipSetKey(bh.Parents...).Bytes()))
	{
		// time-offset mining faults (2 blocks with the same parents)
		timeOffsetWitness, timeOffsetFault, err := checkFault(ctx, f.byParents, parentsKey, bh, "time-offset mining faults")
		if err != nil {
			return cid.Undef, false, xerrors.Errorf("check time-offset mining faults: %w", err)
		}

		if timeOffsetFault {
			return timeOffsetWitness, timeOffsetFault, nil
		}
	}

	{
		// parent-grinding fault (didn't mine on top of our own block)

		// First check if we have mined a block on the parent epoch
		parentEpochKey := ds.NewKey(fmt.Sprintf("/%s/%d", bh.Miner, parentEpoch))
		have, err := f.byEpoch.Has(ctx, parentEpochKey)
		if err != nil {
			return cid.Undef, false, xerrors.Errorf("failed to read from db: %w", err)
		}

		if have {
			// If we had, make sure it's in our parent tipset
			cidb, err := f.byEpoch.Get(ctx, parentEpochKey)
			if err != nil {
				return cid.Undef, false, xerrors.Errorf("getting other block cid: %w", err)
			}

			_, parent, err := cid.CidFromBytes(cidb)
			if err != nil {
				return cid.Undef, false, xerrors.Errorf("failed to read cid from bytes: %w", err)
			}

			var found bool
			for _, c := range bh.Parents {
				if c.Equals(parent) {
					found = true
				}
			}

			if !found {
				return parent, true, nil
			}
		}
	}

	if err := f.byParents.Put(ctx, parentsKey, bh.Cid().Bytes()); err != nil {
		return cid.Undef, false, xerrors.Errorf("putting byEpoch entry: %w", err)
	}

	if err := f.byEpoch.Put(ctx, epochKey, bh.Cid().Bytes()); err != nil {
		return cid.Undef, false, xerrors.Errorf("putting byEpoch entry: %w", err)
	}

	return cid.Undef, false, nil
}

func checkFault(ctx context.Context, t ds.Datastore, key ds.Key, bh *types.BlockHeader, faultType string) (cid.Cid, bool, error) {
	fault, err := t.Has(ctx, key)
	if err != nil {
		return cid.Undef, false, xerrors.Errorf("failed to read from datastore: %w", err)
	}

	if fault {
		cidb, err := t.Get(ctx, key)
		if err != nil {
			return cid.Undef, false, xerrors.Errorf("getting other block cid: %w", err)
		}

		_, other, err := cid.CidFromBytes(cidb)
		if err != nil {
			return cid.Undef, false, xerrors.Errorf("failed to read cid of other block: %w", err)
		}

		if other == bh.Cid() {
			return cid.Undef, false, nil
		}

		return other, true, nil
	}

	return cid.Undef, false, nil
}
