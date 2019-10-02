package deals

import (
	"context"
	"time"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/stmgr"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/lib/cborrpc"
	datastore "github.com/ipfs/go-datastore"
	cbor "github.com/ipfs/go-ipld-cbor"
	inet "github.com/libp2p/go-libp2p-core/network"
	"golang.org/x/xerrors"
)

func (h *Handler) SetPrice(p types.BigInt, ttlsecs int64) error {
	h.askLk.Lock()
	defer h.askLk.Unlock()

	var seqno uint64
	if h.ask != nil {
		seqno = h.ask.Ask.SeqNo + 1
	}

	now := time.Now().Unix()
	ask := &types.StorageAsk{
		Price:        p,
		Timestamp:    now,
		Expiry:       now + ttlsecs,
		Miner:        h.actor,
		SeqNo:        seqno,
		MinPieceSize: h.minPieceSize,
	}

	ssa, err := h.signAsk(ask)
	if err != nil {
		return err
	}

	return h.saveAsk(ssa)
}

func (h *Handler) getAsk(m address.Address) *types.SignedStorageAsk {
	h.askLk.Lock()
	defer h.askLk.Unlock()
	if m != h.actor {
		return nil
	}

	return h.ask
}

func (h *Handler) HandleAskStream(s inet.Stream) {
	defer s.Close()
	var ar AskRequest
	if err := cborrpc.ReadCborRPC(s, &ar); err != nil {
		log.Errorf("failed to read AskRequest from incoming stream: %s", err)
		return
	}

	resp := h.processAskRequest(&ar)

	if err := cborrpc.WriteCborRPC(s, resp); err != nil {
		log.Errorf("failed to write ask response: %s", err)
		return
	}
}

func (h *Handler) processAskRequest(ar *AskRequest) *AskResponse {
	return &AskResponse{
		Ask: h.getAsk(ar.Miner),
	}
}

var bestAskKey = datastore.NewKey("latest-ask")

func (h *Handler) tryLoadAsk() error {
	h.askLk.Lock()
	defer h.askLk.Unlock()

	err := h.loadAsk()
	if err != nil {
		if xerrors.Is(err, datastore.ErrNotFound) {
			log.Warn("no previous ask found, miner will not accept deals until a price is set")
			return nil
		}
		return err
	}

	return nil
}

func (h *Handler) loadAsk() error {
	askb, err := h.ds.Get(datastore.NewKey("latest-ask"))
	if err != nil {
		return xerrors.Errorf("failed to load most recent ask from disk: %w", err)
	}

	var ssa types.SignedStorageAsk
	if err := cbor.DecodeInto(askb, &ssa); err != nil {
		return err
	}

	h.ask = &ssa
	return nil
}

func (h *Handler) signAsk(a *types.StorageAsk) (*types.SignedStorageAsk, error) {
	b, err := cbor.DumpObject(a)
	if err != nil {
		return nil, err
	}

	worker, err := h.getWorker(h.actor)
	if err != nil {
		return nil, xerrors.Errorf("failed to get worker to sign ask: %w", err)
	}

	sig, err := h.full.WalletSign(context.TODO(), worker, b)
	if err != nil {
		return nil, err
	}

	return &types.SignedStorageAsk{
		Ask:       a,
		Signature: sig,
	}, nil
}

func (h *Handler) saveAsk(a *types.SignedStorageAsk) error {
	b, err := cbor.DumpObject(a)
	if err != nil {
		return err
	}

	if err := h.ds.Put(bestAskKey, b); err != nil {
		return err
	}

	h.ask = a
	return nil
}

func (c *Client) checkAskSignature(ask *types.SignedStorageAsk) error {
	tss := c.sm.ChainStore().GetHeaviestTipSet().ParentState()

	w, err := stmgr.GetMinerWorker(context.TODO(), c.sm, tss, ask.Ask.Miner)
	if err != nil {
		return xerrors.Errorf("failed to get worker for miner in ask", err)
	}

	sigb, err := cbor.DumpObject(ask.Ask)
	if err != nil {
		return xerrors.Errorf("failed to re-serialize ask")
	}

	return ask.Signature.Verify(w, sigb)

}
