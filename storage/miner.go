package storage

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/wallet"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log"
	host "github.com/libp2p/go-libp2p-core/host"
	"github.com/pkg/errors"

	"github.com/filecoin-project/go-lotus/lib/sectorbuilder"
)

var log = logging.Logger("storageminer")

type Miner struct {
	api storageMinerApi

	sb *sectorbuilder.SectorBuilder

	w *wallet.Wallet

	maddr address.Address

	worker address.Address

	h host.Host

	ds datastore.Batching
}

type storageMinerApi interface {
	// I think I want this... but this is tricky
	//ReadState(ctx context.Context, addr address.Address) (????, error)

	// Call a read only method on actors (no interaction with the chain required)
	ChainCall(ctx context.Context, msg *types.Message, ts *types.TipSet) (*types.MessageReceipt, error)

	MpoolPush(context.Context, *types.SignedMessage) error
	MpoolGetNonce(context.Context, address.Address) (uint64, error)

	ChainWaitMsg(context.Context, cid.Cid) (*api.MsgWait, error)
	ChainNotify(context.Context) (<-chan *store.HeadChange, error)
}

func NewMiner(api storageMinerApi, addr address.Address, h host.Host, ds datastore.Batching, sb *sectorbuilder.SectorBuilder, w *wallet.Wallet) (*Miner, error) {
	return &Miner{
		api:   api,
		maddr: addr,
		h:     h,
		ds:    ds,
		sb:    sb,
		w:     w,
	}, nil
}

func (m *Miner) Run(ctx context.Context) error {
	if err := m.runPreflightChecks(ctx); err != nil {
		return errors.Wrap(err, "miner preflight checks failed")
	}

	go m.handlePostingSealedSectors(ctx)
	go m.runPoSt(ctx)
	return nil
}

func (m *Miner) handlePostingSealedSectors(ctx context.Context) {
	for {
		select {
		case sinfo, ok := <-m.sb.SealedSectorChan():
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
	ok, err := sectorbuilder.VerifySeal(1024, sinfo.CommR[:], sinfo.CommD[:], sinfo.CommRStar[:], m.maddr, sinfo.SectorID, sinfo.Proof)
	if err != nil {
		log.Error("failed to verify seal we just created: ", err)
	}
	if !ok {
		log.Error("seal we just created failed verification")
	}

	params := &actors.CommitSectorParams{
		SectorID:  types.NewInt(sinfo.SectorID),
		CommD:     sinfo.CommD[:],
		CommR:     sinfo.CommR[:],
		CommRStar: sinfo.CommRStar[:],
		Proof:     sinfo.Proof,
	}
	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return errors.Wrap(aerr, "could not serialize commit sector parameters")
	}

	msg := types.Message{
		To:       m.maddr,
		From:     m.worker,
		Method:   actors.MAMethods.CommitSector,
		Params:   enc,
		Value:    types.NewInt(0), // TODO: need to ensure sufficient collateral
		GasLimit: types.NewInt(1000 /* i dont know help */),
		GasPrice: types.NewInt(1),
	}

	nonce, err := m.api.MpoolGetNonce(ctx, m.worker)
	if err != nil {
		return errors.Wrap(err, "failed to get nonce")
	}

	msg.Nonce = nonce

	data, err := msg.Serialize()
	if err != nil {
		return errors.Wrap(err, "serializing commit sector message")
	}

	sig, err := m.w.Sign(m.worker, data)
	if err != nil {
		return errors.Wrap(err, "signing commit sector message")
	}

	smsg := &types.SignedMessage{
		Message:   msg,
		Signature: *sig,
	}

	if err := m.api.MpoolPush(ctx, smsg); err != nil {
		return errors.Wrap(err, "pushing commit sector message to mpool")
	}

	m.trackCommitSectorMessage(smsg)
	return nil
}

// make sure the message gets included in the chain successfully
func (m *Miner) trackCommitSectorMessage(smsg *types.SignedMessage) {
	log.Warning("not currently tracking commit sector messages")
}

func (m *Miner) runPoSt(ctx context.Context) {
	log.Warning("dont care about posts yet")
}

func (m *Miner) runPreflightChecks(ctx context.Context) error {
	worker, err := m.getWorkerAddr(ctx)
	if err != nil {
		return err
	}

	m.worker = worker

	has, err := m.w.HasKey(worker)
	if err != nil {
		return errors.Wrap(err, "failed to check wallet for worker key")
	}

	if !has {
		return errors.New("key for worker not found in local wallet")
	}

	log.Infof("starting up miner %s, worker addr %s", m.maddr, m.worker)
	return nil
}

func (m *Miner) getWorkerAddr(ctx context.Context) (address.Address, error) {
	msg := &types.Message{
		To:     m.maddr,
		From:   m.maddr, // it doesnt like it if we dont give it a from... probably should fix that
		Method: actors.MAMethods.GetWorkerAddr,
		Params: actors.EmptyStructCBOR,
	}

	recpt, err := m.api.ChainCall(ctx, msg, nil)
	if err != nil {
		return address.Undef, errors.Wrapf(err, "calling getWorker(%s)", m.maddr)
	}

	if recpt.ExitCode != 0 {
		return address.Undef, fmt.Errorf("failed to call getWorker(%s): return %d", m.maddr, recpt.ExitCode)
	}

	return address.NewFromBytes(recpt.Return)
}
