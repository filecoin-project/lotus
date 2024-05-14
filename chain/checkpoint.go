package chain

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
)

func (syncer *Syncer) SyncCheckpoint(ctx context.Context, tsk types.TipSetKey) error {
	if tsk == types.EmptyTSK {
		return xerrors.Errorf("called with empty tsk")
	}

	ts, err := syncer.ChainStore().LoadTipSet(ctx, tsk)
	if err != nil {
		tss, err := syncer.Exchange.GetBlocks(ctx, tsk, 1)
		if err != nil {
			return xerrors.Errorf("failed to fetch tipset: %w", err)
		} else if len(tss) != 1 {
			return xerrors.Errorf("expected 1 tipset, got %d", len(tss))
		}
		ts = tss[0]
	}

	hts := syncer.ChainStore().GetHeaviestTipSet()
	if !hts.Equals(ts) {
		if anc, err := syncer.store.IsAncestorOf(ctx, ts, hts); err != nil {
			return xerrors.Errorf("failed to walk the chain when checkpointing: %w", err)
		} else if !anc {
			if err := syncer.collectChain(ctx, ts, hts, true); err != nil {
				return xerrors.Errorf("failed to collect chain for checkpoint: %w", err)
			}
		} // else new checkpoint is on the current chain, we definitely have the tipsets.
	} // else current head, no need to switch.

	if err := syncer.ChainStore().SetCheckpoint(ctx, ts); err != nil {
		return xerrors.Errorf("failed to set the chain checkpoint: %w", err)
	}

	return nil
}
