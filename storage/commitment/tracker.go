package commitment

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	dsq "github.com/ipfs/go-datastore/query"
)

var log = logging.Logger("commitment")

func init() {
	cbor.RegisterCborType(commitment{})
}

var commitmentDsPrefix = datastore.NewKey("/commitments")

type Tracker struct {
	commitments datastore.Datastore

	lk sync.Mutex

	waits map[datastore.Key]chan struct{}
}

func NewTracker(ds dtypes.MetadataDS) *Tracker {
	return &Tracker{
		commitments: namespace.Wrap(ds, commitmentDsPrefix),
		waits:       map[datastore.Key]chan struct{}{},
	}
}

type commitment struct {
	DealIDs []uint64
	Msg     cid.Cid
}

func commitmentKey(miner address.Address, sectorId uint64) datastore.Key {
	return commitmentDsPrefix.ChildString(miner.String()).ChildString(fmt.Sprintf("%d", sectorId))
}

func (ct *Tracker) TrackCommitSectorMsg(miner address.Address, sectorId uint64, commitMsg cid.Cid) error {
	key := commitmentKey(miner, sectorId)

	ct.lk.Lock()
	defer ct.lk.Unlock()

	tracking, err := ct.commitments.Get(key)
	switch err {
	case datastore.ErrNotFound:
		comm := &commitment{Msg: commitMsg}
		commB, err := cbor.DumpObject(comm)
		if err != nil {
			return err
		}

		if err := ct.commitments.Put(key, commB); err != nil {
			return err
		}

		waits, ok := ct.waits[key]
		if ok {
			close(waits)
			delete(ct.waits, key)
		}
		return nil
	case nil:
		var comm commitment
		if err := cbor.DecodeInto(tracking, &comm); err != nil {
			return err
		}

		if !comm.Msg.Equals(commitMsg) {
			return xerrors.Errorf("commitment tracking for miner %s, sector %d: already tracking %s, got another commitment message: %s", miner, sectorId, comm.Msg, commitMsg)
		}

		log.Warnf("commitment.TrackCommitSectorMsg called more than once for miner %s, sector %d, message %s", miner, sectorId, commitMsg)
		return nil
	default:
		return err
	}
}

func (ct *Tracker) WaitCommit(ctx context.Context, miner address.Address, sectorId uint64) (cid.Cid, error) {
	key := commitmentKey(miner, sectorId)

	ct.lk.Lock()

	tracking, err := ct.commitments.Get(key)
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

	ct.lk.Unlock()

	select {
	case <-wait:
		tracking, err := ct.commitments.Get(key)
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

func (ct *Tracker) List() ([]api.SectorCommitment, error) {
	out := make([]api.SectorCommitment, 0)

	ct.lk.Lock()
	defer ct.lk.Unlock()

	res, err := ct.commitments.Query(dsq.Query{})
	if err != nil {
		return nil, err
	}
	defer res.Close()

	for {
		res, ok := res.NextSync()
		if !ok {
			break
		}

		if res.Error != nil {
			return nil, xerrors.Errorf("iterating commitments: %w", err)
		}

		parts := strings.Split(res.Key, "/")
		if len(parts) != 4 {
			return nil, xerrors.Errorf("expected commitment key to be 4 parts, Key %s", res.Key)
		}

		miner, err := address.NewFromString(parts[2])
		if err != nil {
			return nil, xerrors.Errorf("parsing miner address: %w", err)
		}

		sectorID, err := strconv.ParseInt(parts[3], 10, 64)
		if err != nil {
			return nil, xerrors.Errorf("parsing sector id: %w", err)
		}

		var comm commitment
		if err := cbor.DecodeInto(res.Value, &comm); err != nil {
			return nil, xerrors.Errorf("decoding commitment %s (`% X`): %w", res.Key, res.Value, err)
		}

		out = append(out, api.SectorCommitment{
			SectorID:  uint64(sectorID),
			Miner:     miner,
			CommitMsg: comm.Msg,
			DealIDs:   comm.DealIDs,
		})
	}

	return out, nil
}
