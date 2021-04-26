package chain

import (
	"context"
	"encoding/json"

	"github.com/filecoin-project/lotus/chain/types"

	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/ipfs/go-datastore"
	"golang.org/x/xerrors"
)

var CheckpointKey = datastore.NewKey("/chain/checks")

func loadCheckpoint(ds dtypes.MetadataDS) (types.TipSetKey, error) {
	haveChks, err := ds.Has(CheckpointKey)
	if err != nil {
		return types.EmptyTSK, err
	}

	if !haveChks {
		return types.EmptyTSK, nil
	}

	tskBytes, err := ds.Get(CheckpointKey)
	if err != nil {
		return types.EmptyTSK, err
	}

	var tsk types.TipSetKey
	err = json.Unmarshal(tskBytes, &tsk)
	if err != nil {
		return types.EmptyTSK, err
	}

	return tsk, err
}

func (syncer *Syncer) SyncCheckpoint(ctx context.Context, tsk types.TipSetKey) error {
	if tsk == types.EmptyTSK {
		return xerrors.Errorf("called with empty tsk")
	}

	ts, err := syncer.ChainStore().LoadTipSet(tsk)
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
	anc, err := syncer.ChainStore().IsAncestorOf(ts, hts)
	if err != nil {
		return xerrors.Errorf("cannot determine whether checkpoint tipset is in main-chain: %w", err)
	}
	if !hts.Equals(ts) && !anc {
		if err := syncer.collectChain(ctx, ts, hts); err != nil {
			return xerrors.Errorf("failed to collect chain for checkpoint: %w", err)
		}
		if err := syncer.ChainStore().SetHead(ts); err != nil {
			return xerrors.Errorf("failed to set the chain head: %w", err)
		}
	}

	syncer.checkptLk.Lock()
	defer syncer.checkptLk.Unlock()

	tskBytes, err := json.Marshal(tsk)
	if err != nil {
		return err
	}

	err = syncer.ds.Put(CheckpointKey, tskBytes)
	if err != nil {
		return err
	}

	// TODO: This is racy. as there may be a concurrent sync in progress.
	// The only real solution is to checkpoint inside the chainstore, not here.
	syncer.checkpt = tsk

	return nil
}

func (syncer *Syncer) GetCheckpoint() types.TipSetKey {
	syncer.checkptLk.Lock()
	defer syncer.checkptLk.Unlock()
	return syncer.checkpt
}
