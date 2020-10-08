package chain

import (
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

func (syncer *Syncer) SetCheckpoint(tsk types.TipSetKey) error {
	if tsk == types.EmptyTSK {
		return xerrors.Errorf("called with empty tsk")
	}

	syncer.checkptLk.Lock()
	defer syncer.checkptLk.Unlock()

	ts, err := syncer.ChainStore().LoadTipSet(tsk)
	if err != nil {
		return xerrors.Errorf("cannot find tipset: %w", err)
	}

	hts := syncer.ChainStore().GetHeaviestTipSet()
	anc, err := syncer.ChainStore().IsAncestorOf(ts, hts)
	if err != nil {
		return xerrors.Errorf("cannot determine whether checkpoint tipset is in main-chain: %w", err)
	}

	if !hts.Equals(ts) && !anc {
		return xerrors.Errorf("cannot mark tipset as checkpoint, since it isn't in the main-chain: %w", err)
	}

	tskBytes, err := json.Marshal(tsk)
	if err != nil {
		return err
	}

	err = syncer.ds.Put(CheckpointKey, tskBytes)
	if err != nil {
		return err
	}

	syncer.checkpt = tsk

	return nil
}

func (syncer *Syncer) GetCheckpoint() types.TipSetKey {
	syncer.checkptLk.Lock()
	defer syncer.checkptLk.Unlock()
	return syncer.checkpt
}
