package storage

import (
	"context"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"
	"sync"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/events"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/lib/sectorbuilder"
	"github.com/filecoin-project/go-lotus/storage/commitment"
	"github.com/filecoin-project/go-lotus/storage/sector"
)

var log = logging.Logger("storageminer")

const PoStConfidence = 1

type Miner struct {
	api    storageMinerApi
	events *events.Events

	secst *sector.Store
	commt *commitment.Tracker

	maddr address.Address

	worker address.Address

	h host.Host

	ds datastore.Batching

	schedLk   sync.Mutex
	postSched uint64
}

type storageMinerApi interface {
	// I think I want this... but this is tricky
	//ReadState(ctx context.Context, addr address.Address) (????, error)

	// Call a read only method on actors (no interaction with the chain required)
	StateCall(ctx context.Context, msg *types.Message, ts *types.TipSet) (*types.MessageReceipt, error)
	StateMinerWorker(context.Context, address.Address, *types.TipSet) (address.Address, error)
	StateMinerProvingPeriodEnd(context.Context, address.Address, *types.TipSet) (uint64, error)
	StateMinerProvingSet(context.Context, address.Address, *types.TipSet) ([]*api.SectorInfo, error)

	MpoolPushMessage(context.Context, *types.Message) (*types.SignedMessage, error)

	ChainHead(context.Context) (*types.TipSet, error)
	ChainWaitMsg(context.Context, cid.Cid) (*api.MsgWait, error)
	ChainNotify(context.Context) (<-chan []*store.HeadChange, error)
	ChainGetRandomness(context.Context, *types.TipSet, []*types.Ticket, int) ([]byte, error)
	ChainGetTipSetByHeight(context.Context, uint64, *types.TipSet) (*types.TipSet, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*api.BlockMessages, error)

	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletHas(context.Context, address.Address) (bool, error)
}

func NewMiner(api storageMinerApi, addr address.Address, h host.Host, ds datastore.Batching, secst *sector.Store, commt *commitment.Tracker) (*Miner, error) {
	return &Miner{
		api: api,

		maddr: addr,
		h:     h,
		ds:    ds,
		secst: secst,
		commt: commt,
	}, nil
}

func (m *Miner) Run(ctx context.Context) error {
	if err := m.runPreflightChecks(ctx); err != nil {
		return errors.Wrap(err, "miner preflight checks failed")
	}

	m.events = events.NewEvents(ctx, m.api)

	ts, err := m.api.ChainHead(ctx)
	if err != nil {
		return err
	}

	go m.handlePostingSealedSectors(ctx)
	go m.schedulePoSt(ctx, ts)
	return nil
}

func (m *Miner) handlePostingSealedSectors(ctx context.Context) {
	incoming := m.secst.Incoming()
	defer m.secst.CloseIncoming(incoming)

	for {
		select {
		case sinfo, ok := <-incoming:
			if !ok {
				// TODO: set some state variable so that this state can be
				// visible via some status command
				log.Warning("sealed sector channel closed, aborting process")
				return
			}

			if err := m.commitSector(ctx, sinfo); err != nil {
				log.Errorf("failed to commit sector: %s", err)
				continue
			}

		case <-ctx.Done():
			log.Warning("exiting seal posting routine")
			return
		}
	}
}

func (m *Miner) commitSector(ctx context.Context, sinfo sectorbuilder.SectorSealingStatus) error {
	log.Info("committing sector")

	ok, err := sectorbuilder.VerifySeal(build.SectorSize, sinfo.CommR[:], sinfo.CommD[:], sinfo.CommRStar[:], m.maddr, sinfo.SectorID, sinfo.Proof)
	if err != nil {
		log.Error("failed to verify seal we just created: ", err)
	}
	if !ok {
		log.Error("seal we just created failed verification")
	}

	params := &actors.CommitSectorParams{
		SectorID:  sinfo.SectorID,
		CommD:     sinfo.CommD[:],
		CommR:     sinfo.CommR[:],
		CommRStar: sinfo.CommRStar[:],
		Proof:     sinfo.Proof,
	}
	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return errors.Wrap(aerr, "could not serialize commit sector parameters")
	}

	msg := &types.Message{
		To:       m.maddr,
		From:     m.worker,
		Method:   actors.MAMethods.CommitSector,
		Params:   enc,
		Value:    types.NewInt(0), // TODO: need to ensure sufficient collateral
		GasLimit: types.NewInt(100000 /* i dont know help */),
		GasPrice: types.NewInt(1),
	}

	smsg, err := m.api.MpoolPushMessage(ctx, msg)
	if err != nil {
		return errors.Wrap(err, "pushing message to mpool")
	}

	if err := m.commt.TrackCommitSectorMsg(m.maddr, sinfo.SectorID, smsg.Cid()); err != nil {
		return errors.Wrap(err, "tracking sector commitment")
	}

	go func() {
		_, err := m.api.ChainWaitMsg(ctx, smsg.Cid())
		if err != nil {
			return
		}

		m.schedulePoSt(ctx, nil)
	}()

	return nil
}

func (m *Miner) schedulePoSt(ctx context.Context, baseTs *types.TipSet) {
	ppe, err := m.api.StateMinerProvingPeriodEnd(ctx, m.maddr, baseTs)
	if err != nil {
		log.Errorf("failed to get proving period end for miner: %s", err)
		return
	}

	if ppe == 0 {
		log.Errorf("Proving period end == 0")
		// TODO: we probably want to call schedulePoSt after the first commitSector call
		return
	}

	m.schedLk.Lock()

	if m.postSched >= ppe {
		log.Warn("schedulePoSt already called for proving period >= %d", m.postSched)
		m.schedLk.Unlock()
		return
	}
	m.postSched = ppe
	m.schedLk.Unlock()

	log.Infof("Scheduling post at height %d", ppe-build.PoSTChallangeTime)
	err = m.events.ChainAt(m.startPost, func(ts *types.TipSet) error { // Revert
		// TODO: Cancel post
		return nil
	}, PoStConfidence, ppe-build.PoSTChallangeTime)
	if err != nil {
		// TODO: This is BAD, figure something out
		log.Errorf("scheduling PoSt failed: %s", err)
		return
	}
}

func (m *Miner) startPost(ts *types.TipSet, curH uint64) error {
	log.Info("starting PoSt computation")

	head, err := m.api.ChainHead(context.TODO())
	if err != nil {
		return err
	}

	postWaitCh, _, err := m.maybeDoPost(context.TODO(), head)
	if err != nil {
		return err
	}

	if postWaitCh == nil {
		return errors.New("PoSt didn't start")
	}

	go func() {
		err := <-postWaitCh
		if err != nil {
			log.Errorf("got error back from postWaitCh: %s", err)
			return
		}

		log.Infof("post successfully submitted")

		m.schedulePoSt(context.TODO(), ts)
	}()
	return nil
}

func (m *Miner) maybeDoPost(ctx context.Context, ts *types.TipSet) (<-chan error, *types.BlockHeader, error) {
	ppe, err := m.api.StateMinerProvingPeriodEnd(ctx, m.maddr, ts)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to get proving period end for miner: %w", err)
	}

	if ppe < ts.Height() {
		log.Warnf("skipping post, supplied tipset too high: ppe=%d, ts.H=%d", ppe, ts.Height())
		return nil, nil, nil
	}

	sset, err := m.api.StateMinerProvingSet(ctx, m.maddr, ts)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to get proving set for miner: %w", err)
	}

	r, err := m.api.ChainGetRandomness(ctx, ts, nil, int(ts.Height()-ppe+build.ProvingPeriodDuration)) // TODO: review: check math
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to get chain randomness for post: %w", err)
	}

	sourceTs, err := m.api.ChainGetTipSetByHeight(ctx, ppe-build.ProvingPeriodDuration, ts)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to get post start tipset: %w", err)
	}

	ret := make(chan error, 1)
	go func() {
		log.Info("running PoSt computation")
		var faults []uint64
		proof, err := m.secst.RunPoSt(ctx, sset, r, faults)
		if err != nil {
			ret <- xerrors.Errorf("running post failed: %w", err)
			return
		}

		log.Info("submitting PoSt")

		params := &actors.SubmitPoStParams{
			Proof:   proof,
			DoneSet: types.BitFieldFromSet(sectorIdList(sset)),
		}

		enc, aerr := actors.SerializeParams(params)
		if aerr != nil {
			ret <- xerrors.Errorf("could not serialize submit post parameters: %w", err)
			return
		}

		msg := &types.Message{
			To:       m.maddr,
			From:     m.worker,
			Method:   actors.MAMethods.SubmitPoSt,
			Params:   enc,
			Value:    types.NewInt(0),
			GasLimit: types.NewInt(100000 /* i dont know help */),
			GasPrice: types.NewInt(1),
		}

		_, err = m.api.MpoolPushMessage(ctx, msg)
		if err != nil {
			ret <- xerrors.Errorf("pushing message to mpool: %w", err)
			return
		}

		// make sure it succeeds...
		// m.api.ChainWaitMsg()

	}()

	return ret, sourceTs.MinTicketBlock(), nil
}

func sectorIdList(si []*api.SectorInfo) []uint64 {
	out := make([]uint64, len(si))
	for i, s := range si {
		out[i] = s.SectorID
	}
	return out
}

func (m *Miner) runPreflightChecks(ctx context.Context) error {
	worker, err := m.api.StateMinerWorker(ctx, m.maddr, nil)
	if err != nil {
		return err
	}

	m.worker = worker

	has, err := m.api.WalletHas(ctx, worker)
	if err != nil {
		return errors.Wrap(err, "failed to check wallet for worker key")
	}

	if !has {
		return errors.New("key for worker not found in local wallet")
	}

	log.Infof("starting up miner %s, worker addr %s", m.maddr, m.worker)
	return nil
}
