package commitment

import (
	"context"
	"fmt"
	"sync"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/node/modules/dtypes"
)

var log = logging.Logger("commitment")

func init() {
	cbor.RegisterCborType(commitment{})
}

var commitmentDsPrefix = datastore.NewKey("/commitments")

type Tracker struct {
	commitDs datastore.Datastore

	lk sync.Mutex

	waits map[datastore.Key]chan struct{}
}

func NewTracker(ds dtypes.MetadataDS) *Tracker {
	return &Tracker{
		commitDs: namespace.Wrap(ds, commitmentDsPrefix),
		waits: map[datastore.Key]chan struct{}{},
	}
}

type commitment struct {
	Msg cid.Cid
}

func commitmentKey(miner address.Address, sectorId uint64) datastore.Key {
	return commitmentDsPrefix.ChildString(miner.String()).ChildString(fmt.Sprintf("%d", sectorId))
}

func (ct *Tracker) TrackCommitSectorMsg(sectorId uint64, smsg *types.SignedMessage) error {
	mcid := smsg.Cid()
	key := commitmentKey(smsg.Message.From, sectorId)

	ct.lk.Lock()
	defer ct.lk.Unlock()

	tracking, err := ct.commitDs.Get(key)
	if err != datastore.ErrNotFound {
		if err != nil {
			return err
		}

		var comm commitment
		if err := cbor.DecodeInto(tracking, &comm); err != nil {
			return err
		}

		if !comm.Msg.Equals(mcid) {
			return xerrors.Errorf("commitment tracking for miner %s, sector %d: already tracking %s, got another commitment message: %s", smsg.Message.From, sectorId, comm.Msg, mcid)
		}

		log.Warnf("commitment.TrackCommitSectorMsg called more than once for miner %s, sector %d, message %s", smsg.Message.From, sectorId, mcid)
		return nil
	}

	comm := &commitment{Msg:mcid}
	commB, err := cbor.DumpObject(comm)
	if err != nil {
		return err
	}

	if err := ct.commitDs.Put(key, commB); err != nil {
		return err
	}

	waits, ok := ct.waits[key]
	if ok {
		close(waits)
		delete(ct.waits, key)
	}

	return nil
}

func (ct *Tracker) WaitCommit(ctx context.Context, miner address.Address, sectorId uint64) (cid.Cid, error) {
	key := commitmentKey(miner, sectorId)

	ct.lk.Lock()

	tracking, err := ct.commitDs.Get(key)
	if err != datastore.ErrNotFound {
		ct.lk.Unlock()

		if err != nil {
			return cid.Undef, err
		}

		var comm commitment
		if err := cbor.DecodeInto(tracking, &comm); err != nil {
			return cid.Undef, err
		}

		return comm.Msg, nil
	}

	wait, ok := ct.waits[key]
	if !ok {
		wait = make(chan struct{})
		ct.waits[key] = wait
	}
	select {
	case <-wait:
		tracking, err := ct.commitDs.Get(key)
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to get commitment after waiting: %w", err)
		}

		var comm commitment
		if err := cbor.DecodeInto(tracking, &comm); err != nil {
			return cid.Undef, err
		}

		return comm.Msg, nil
	case <-ctx.Done():
		return cid.Undef, ctx.Err()
	}
}
