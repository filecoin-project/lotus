package retrievalimpl

import (
	"context"
	"reflect"
	"sync"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/cborutil"
	retrievalmarket "github.com/filecoin-project/lotus/retrieval"
)

var log = logging.Logger("retrieval")

type client struct {
	h    host.Host
	bs   blockstore.Blockstore
	node retrievalmarket.RetrievalClientNode
	// The parameters should be replaced by RetrievalClientNode

	nextDealLk  sync.Mutex
	nextDealID  retrievalmarket.DealID
	subscribers []retrievalmarket.ClientSubscriber
}

// NewClient creates a new retrieval client
func NewClient(h host.Host, bs blockstore.Blockstore, node retrievalmarket.RetrievalClientNode) retrievalmarket.RetrievalClient {
	return &client{h: h, bs: bs, node: node}
}

// V0

// TODO: Implement for retrieval provider V0 epic
// https://github.com/filecoin-project/go-retrieval-market-project/issues/12
func (c *client) FindProviders(pieceCID []byte) []retrievalmarket.RetrievalPeer {
	panic("not implemented")
}

// TODO: Update to match spec for V0 epic
// https://github.com/filecoin-project/go-retrieval-market-project/issues/8
func (c *client) Query(ctx context.Context, p retrievalmarket.RetrievalPeer, pieceCID []byte, params retrievalmarket.QueryParams) (retrievalmarket.QueryResponse, error) {
	cid, err := cid.Cast(pieceCID)
	if err != nil {
		log.Warn(err)
		return retrievalmarket.QueryResponseUndefined, err
	}

	s, err := c.h.NewStream(ctx, p.ID, retrievalmarket.QueryProtocolID)
	if err != nil {
		log.Warn(err)
		return retrievalmarket.QueryResponseUndefined, err
	}
	defer s.Close()

	err = cborutil.WriteCborRPC(s, &OldQuery{
		Piece: cid,
	})
	if err != nil {
		log.Warn(err)
		return retrievalmarket.QueryResponseUndefined, err
	}

	var oldResp OldQueryResponse
	if err := oldResp.UnmarshalCBOR(s); err != nil {
		log.Warn(err)
		return retrievalmarket.QueryResponseUndefined, err
	}

	resp := retrievalmarket.QueryResponse{
		Status:          retrievalmarket.QueryResponseStatus(oldResp.Status),
		Size:            oldResp.Size,
		MinPricePerByte: types.BigDiv(oldResp.MinPrice, types.NewInt(oldResp.Size)),
	}
	return resp, nil
}

// TODO: Update to match spec for V0 Epic:
// https://github.com/filecoin-project/go-retrieval-market-project/issues/9
func (c *client) Retrieve(ctx context.Context, pieceCID []byte, params retrievalmarket.Params, totalFunds types.BigInt, miner peer.ID, clientWallet retrievalmarket.Address, minerWallet retrievalmarket.Address) retrievalmarket.DealID {
	/* The implementation of this function is just wrapper for the old code which retrieves UnixFS pieces
	-- it will be replaced when we do the V0 implementation of the module */
	c.nextDealLk.Lock()
	c.nextDealID++
	dealID := c.nextDealID
	c.nextDealLk.Unlock()

	dealState := retrievalmarket.ClientDealState{
		DealProposal: retrievalmarket.DealProposal{
			PieceCID: pieceCID,
			ID:       dealID,
			Params:   params,
		},
		Status: retrievalmarket.DealStatusFailed,
		Sender: miner,
	}

	go func() {
		evt := retrievalmarket.ClientEventError
		converted, err := cid.Cast(pieceCID)

		if err == nil {
			err = c.retrieveUnixfs(ctx, converted, types.BigDiv(totalFunds, params.PricePerByte).Uint64(), totalFunds, miner, clientWallet, minerWallet)
			if err == nil {
				evt = retrievalmarket.ClientEventComplete
				dealState.Status = retrievalmarket.DealStatusCompleted
			}
		}

		c.notifySubscribers(evt, dealState)
	}()

	return dealID
}

// unsubscribeAt returns a function that removes an item from the subscribers list by comparing
// their reflect.ValueOf before pulling the item out of the slice.  Does not preserve order.
// Subsequent, repeated calls to the func with the same Subscriber are a no-op.
func (c *client) unsubscribeAt(sub retrievalmarket.ClientSubscriber) retrievalmarket.Unsubscribe {
	return func() {
		curLen := len(c.subscribers)
		for i, el := range c.subscribers {
			if reflect.ValueOf(sub) == reflect.ValueOf(el) {
				c.subscribers[i] = c.subscribers[curLen-1]
				c.subscribers = c.subscribers[:curLen-1]
				return
			}
		}
	}
}

func (c *client) notifySubscribers(evt retrievalmarket.ClientEvent, ds retrievalmarket.ClientDealState) {
	for _, cb := range c.subscribers {
		cb(evt, ds)
	}
}

func (c *client) SubscribeToEvents(subscriber retrievalmarket.ClientSubscriber) retrievalmarket.Unsubscribe {
	c.subscribers = append(c.subscribers, subscriber)
	return c.unsubscribeAt(subscriber)
}

// V1
func (c *client) AddMoreFunds(id retrievalmarket.DealID, amount types.BigInt) error {
	panic("not implemented")
}

func (c *client) CancelDeal(id retrievalmarket.DealID) error {
	panic("not implemented")
}

func (c *client) RetrievalStatus(id retrievalmarket.DealID) {
	panic("not implemented")
}

func (c *client) ListDeals() map[retrievalmarket.DealID]retrievalmarket.ClientDealState {
	panic("not implemented")
}

type clientStream struct {
	node   retrievalmarket.RetrievalClientNode
	stream network.Stream
	peeker cbg.BytePeeker

	root   cid.Cid
	size   types.BigInt
	offset uint64

	paych       retrievalmarket.Address
	lane        uint64
	total       types.BigInt
	transferred types.BigInt

	windowSize uint64 // how much we "trust" the peer
	verifier   BlockVerifier
	bs         blockstore.Blockstore
}

/* This is the old retrieval code that is NOT spec compliant */

// C > S
//
// Offset MUST be aligned on chunking boundaries, size is rounded up to leaf size
//
// > DealProposal{Mode: Unixfs0, RootCid, Offset, Size, Payment(nil if free)}
// < Resp{Accept}
// < ..(Intermediate Block)
// < ..Blocks
// < ..(Intermediate Block)
// < ..Blocks
// > DealProposal(...)
// < ...
func (c *client) retrieveUnixfs(ctx context.Context, root cid.Cid, size uint64, total types.BigInt, miner peer.ID, client, minerAddr retrievalmarket.Address) error {
	s, err := c.h.NewStream(ctx, miner, retrievalmarket.ProtocolID)
	if err != nil {
		return err
	}
	defer s.Close()

	initialOffset := uint64(0) // TODO: Check how much data we have locally
	// TODO: Support in handler
	// TODO: Allow client to specify this

	paych, err := c.node.GetOrCreatePaymentChannel(ctx, client, minerAddr, total)
	if err != nil {
		return xerrors.Errorf("getting payment channel: %w", err)
	}
	lane, err := c.node.AllocateLane(paych)
	if err != nil {
		return xerrors.Errorf("allocating payment lane: %w", err)
	}

	cst := clientStream{
		node:   c.node,
		stream: s,
		peeker: cbg.GetPeeker(s),

		root:   root,
		size:   types.NewInt(size),
		offset: initialOffset,

		paych:       paych,
		lane:        lane,
		total:       total,
		transferred: types.NewInt(0),

		windowSize: build.UnixfsChunkSize,
		verifier:   &UnixFs0Verifier{Root: root},
		bs:         c.bs,
	}

	for cst.offset != size+initialOffset {
		toFetch := cst.windowSize
		if toFetch+cst.offset > size {
			toFetch = size - cst.offset
		}
		log.Infof("Retrieve %dB @%d", toFetch, cst.offset)

		err := cst.doOneExchange(ctx, toFetch)
		if err != nil {
			return xerrors.Errorf("retrieval exchange: %w", err)
		}

		cst.offset += toFetch
	}
	log.Info("RETRIEVE SUCCESSFUL")
	return nil
}

func (cst *clientStream) doOneExchange(ctx context.Context, toFetch uint64) error {
	payAmount := types.BigDiv(types.BigMul(cst.total, types.NewInt(toFetch)), cst.size)

	payment, err := cst.setupPayment(ctx, payAmount)
	if err != nil {
		return xerrors.Errorf("setting up retrieval payment: %w", err)
	}

	deal := &OldDealProposal{
		Payment: payment,
		Ref:     cst.root,
		Params: RetParams{
			Unixfs0: &Unixfs0Offer{
				Offset: cst.offset,
				Size:   toFetch,
			},
		},
	}

	if err := cborutil.WriteCborRPC(cst.stream, deal); err != nil {
		return err
	}

	var resp OldDealResponse
	if err := cborutil.ReadCborRPC(cst.peeker, &resp); err != nil {
		log.Error(err)
		return err
	}

	if resp.Status != Accepted {
		cst.windowSize = build.UnixfsChunkSize
		// TODO: apply some 'penalty' to miner 'reputation' (needs to be the same in both cases)

		if resp.Status == Error {
			return xerrors.Errorf("storage deal error: %s", resp.Message)
		}
		if resp.Status == Rejected {
			return xerrors.Errorf("storage deal rejected: %s", resp.Message)
		}
		return xerrors.New("storage deal response had no Accepted section")
	}

	log.Info("Retrieval accepted, fetching blocks")

	return cst.fetchBlocks(toFetch)

	// TODO: maybe increase miner window size after success
}

func (cst *clientStream) fetchBlocks(toFetch uint64) error {
	blocksToFetch := (toFetch + build.UnixfsChunkSize - 1) / build.UnixfsChunkSize

	for i := uint64(0); i < blocksToFetch; {
		log.Infof("block %d of %d", i+1, blocksToFetch)

		var block Block
		if err := cborutil.ReadCborRPC(cst.peeker, &block); err != nil {
			return xerrors.Errorf("reading fetchBlock response: %w", err)
		}

		dataBlocks, err := cst.consumeBlockMessage(block)
		if err != nil {
			return xerrors.Errorf("consuming retrieved blocks: %w", err)
		}

		i += dataBlocks
	}

	return nil
}

func (cst *clientStream) consumeBlockMessage(block Block) (uint64, error) {
	prefix, err := cid.PrefixFromBytes(block.Prefix)
	if err != nil {
		return 0, err
	}

	cid, err := prefix.Sum(block.Data)

	blk, err := blocks.NewBlockWithCid(block.Data, cid)
	if err != nil {
		return 0, err
	}

	internal, err := cst.verifier.Verify(context.TODO(), blk)
	if err != nil {
		log.Warnf("block verify failed: %s", err)
		return 0, err
	}

	// TODO: Smarter out, maybe add to filestore automagically
	//  (Also, persist intermediate nodes)
	err = cst.bs.Put(blk)
	if err != nil {
		log.Warnf("block write failed: %s", err)
		return 0, err
	}

	if internal {
		return 0, nil
	}

	return 1, nil
}

func (cst *clientStream) setupPayment(ctx context.Context, toSend types.BigInt) (api.PaymentInfo, error) {
	amount := types.BigAdd(cst.transferred, toSend)

	sv, err := cst.node.CreatePaymentVoucher(ctx, cst.paych, amount, cst.lane)
	if err != nil {
		return api.PaymentInfo{}, err
	}

	cst.transferred = amount

	return api.PaymentInfo{
		Channel:        cst.paych,
		ChannelMessage: nil,
		Vouchers:       []*types.SignedVoucher{sv},
	}, nil
}
