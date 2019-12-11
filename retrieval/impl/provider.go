package retrievalimpl

import (
	"context"
	"io"
	"reflect"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/cborutil"
	retrievalmarket "github.com/filecoin-project/lotus/retrieval"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
)

// RetrMinerAPI are the node functions needed by a retrieval provider
type RetrMinerAPI interface {
	PaychVoucherAdd(context.Context, address.Address, *types.SignedVoucher, []byte, types.BigInt) (types.BigInt, error)
}

type provider struct {

	// TODO: Replace with RetrievalProviderNode & FileStore for https://github.com/filecoin-project/go-retrieval-market-project/issues/9
	sectorBlocks *sectorblocks.SectorBlocks

	// TODO: Replace with RetrievalProviderNode for
	// https://github.com/filecoin-project/go-retrieval-market-project/issues/4
	node retrievalmarket.RetrievalProviderNode

	pricePerByte retrievalmarket.BigInt

	subscribers []retrievalmarket.ProviderSubscriber
}

// NewProvider returns a new retrieval provider
func NewProvider(sblks *sectorblocks.SectorBlocks, node retrievalmarket.RetrievalProviderNode) retrievalmarket.RetrievalProvider {
	return &provider{
		sectorBlocks: sblks,
		node:         node,

		pricePerByte: types.NewInt(2), // TODO: allow setting
	}
}

// Start begins listening for deals on the given host
func (p *provider) Start(host host.Host) {
	host.SetStreamHandler(retrievalmarket.QueryProtocolID, p.handleQueryStream)
	host.SetStreamHandler(retrievalmarket.ProtocolID, p.handleDealStream)
}

// V0
// SetPricePerByte sets the price per byte a miner charges for retrievals
func (p *provider) SetPricePerByte(price retrievalmarket.BigInt) {
	p.pricePerByte = price
}

// SetPaymentInterval sets the maximum number of bytes a a provider will send before
// requesting further payment, and the rate at which that value increases
// TODO: Implement for https://github.com/filecoin-project/go-retrieval-market-project/issues/7
func (p *provider) SetPaymentInterval(paymentInterval uint64, paymentIntervalIncrease uint64) {
	panic("not implemented")
}

// unsubscribeAt returns a function that removes an item from the subscribers list by comparing
// their reflect.ValueOf before pulling the item out of the slice.  Does not preserve order.
// Subsequent, repeated calls to the func with the same Subscriber are a no-op.
func (p *provider) unsubscribeAt(sub retrievalmarket.ProviderSubscriber) retrievalmarket.Unsubscribe {
	return func() {
		curLen := len(p.subscribers)
		for i, el := range p.subscribers {
			if reflect.ValueOf(sub) == reflect.ValueOf(el) {
				p.subscribers[i] = p.subscribers[curLen-1]
				p.subscribers = p.subscribers[:curLen-1]
				return
			}
		}
	}
}

func (p *provider) notifySubscribers(evt retrievalmarket.ProviderEvent, ds retrievalmarket.ProviderDealState) {
	for _, cb := range p.subscribers {
		cb(evt, ds)
	}
}

// SubscribeToEvents listens for events that happen related to client retrievals
// TODO: Implement updates as part of https://github.com/filecoin-project/go-retrieval-market-project/issues/7
func (p *provider) SubscribeToEvents(subscriber retrievalmarket.ProviderSubscriber) retrievalmarket.Unsubscribe {
	p.subscribers = append(p.subscribers, subscriber)
	return p.unsubscribeAt(subscriber)
}

// V1
func (p *provider) SetPricePerUnseal(price retrievalmarket.BigInt) {
	panic("not implemented")
}

func (p *provider) ListDeals() map[retrievalmarket.ProviderDealID]retrievalmarket.ProviderDealState {
	panic("not implemented")
}

func writeErr(stream network.Stream, err error) {
	log.Errorf("Retrieval deal error: %+v", err)
	_ = cborutil.WriteCborRPC(stream, &OldDealResponse{
		Status:  Error,
		Message: err.Error(),
	})
}

// TODO: Update for https://github.com/filecoin-project/go-retrieval-market-project/issues/8
func (p *provider) handleQueryStream(stream network.Stream) {
	defer stream.Close()

	var query OldQuery
	if err := cborutil.ReadCborRPC(stream, &query); err != nil {
		writeErr(stream, err)
		return
	}

	size, err := p.sectorBlocks.GetSize(query.Piece)
	if err != nil && err != sectorblocks.ErrNotFound {
		log.Errorf("Retrieval query: GetRefs: %s", err)
		return
	}

	answer := &OldQueryResponse{
		Status: Unavailable,
	}
	if err == nil {
		answer.Status = Available

		// TODO: get price, look for already unsealed ref to reduce work
		answer.MinPrice = types.BigMul(types.NewInt(uint64(size)), p.pricePerByte)
		answer.Size = uint64(size) // TODO: verify on intermediate
	}

	if err := cborutil.WriteCborRPC(stream, answer); err != nil {
		log.Errorf("Retrieval query: WriteCborRPC: %s", err)
		return
	}
}

type handlerDeal struct {
	p      *provider
	stream network.Stream

	ufsr sectorblocks.UnixfsReader
	open cid.Cid
	at   uint64
	size uint64
}

// TODO: Update for https://github.com/filecoin-project/go-retrieval-market-project/issues/7
func (p *provider) handleDealStream(stream network.Stream) {
	defer stream.Close()

	hnd := &handlerDeal{
		p: p,

		stream: stream,
	}

	var err error
	more := true

	for more {
		more, err = hnd.handleNext() // TODO: 'more' bool
		if err != nil {
			writeErr(stream, err)
			return
		}
	}

}

func (hnd *handlerDeal) handleNext() (bool, error) {
	var deal OldDealProposal
	if err := cborutil.ReadCborRPC(hnd.stream, &deal); err != nil {
		if err == io.EOF { // client sent all deals
			err = nil
		}
		return false, err
	}

	if deal.Params.Unixfs0 == nil {
		return false, xerrors.New("unknown deal type")
	}

	unixfs0 := deal.Params.Unixfs0

	if len(deal.Payment.Vouchers) != 1 {
		return false, xerrors.Errorf("expected one signed voucher, got %d", len(deal.Payment.Vouchers))
	}

	expPayment := types.BigMul(hnd.p.pricePerByte, types.NewInt(deal.Params.Unixfs0.Size))
	if _, err := hnd.p.node.SavePaymentVoucher(context.TODO(), deal.Payment.Channel, deal.Payment.Vouchers[0], nil, expPayment); err != nil {
		return false, xerrors.Errorf("processing retrieval payment: %w", err)
	}

	// If the file isn't open (new deal stream), isn't the right file, or isn't
	// at the right offset, (re)open it
	if hnd.open != deal.Ref || hnd.at != unixfs0.Offset {
		log.Infof("opening file for sending (open '%s') (@%d, want %d)", deal.Ref, hnd.at, unixfs0.Offset)
		if err := hnd.openFile(deal); err != nil {
			return false, err
		}
	}

	if unixfs0.Offset+unixfs0.Size > hnd.size {
		return false, xerrors.Errorf("tried to read too much %d+%d > %d", unixfs0.Offset, unixfs0.Size, hnd.size)
	}

	err := hnd.accept(deal)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (hnd *handlerDeal) openFile(deal OldDealProposal) error {
	unixfs0 := deal.Params.Unixfs0

	if unixfs0.Offset != 0 {
		// TODO: Implement SeekBlock (like ReadBlock) in go-unixfs
		return xerrors.New("sending merkle proofs for nonzero offset not supported yet")
	}
	hnd.at = unixfs0.Offset

	bstore := hnd.p.sectorBlocks.SealedBlockstore(func() error {
		return nil // TODO: approve unsealing based on amount paid
	})

	ds := merkledag.NewDAGService(blockservice.New(bstore, nil))
	rootNd, err := ds.Get(context.TODO(), deal.Ref)
	if err != nil {
		return err
	}

	fsr, err := unixfile.NewUnixfsFile(context.TODO(), ds, rootNd)
	if err != nil {
		return err
	}

	var ok bool
	hnd.ufsr, ok = fsr.(sectorblocks.UnixfsReader)
	if !ok {
		return xerrors.Errorf("file %s didn't implement sectorblocks.UnixfsReader", deal.Ref)
	}

	isize, err := hnd.ufsr.Size()
	if err != nil {
		return err
	}
	hnd.size = uint64(isize)

	hnd.open = deal.Ref

	return nil
}

func (hnd *handlerDeal) accept(deal OldDealProposal) error {
	unixfs0 := deal.Params.Unixfs0

	resp := &OldDealResponse{
		Status: Accepted,
	}
	if err := cborutil.WriteCborRPC(hnd.stream, resp); err != nil {
		log.Errorf("Retrieval query: Write Accepted resp: %s", err)
		return err
	}

	blocksToSend := (unixfs0.Size + build.UnixfsChunkSize - 1) / build.UnixfsChunkSize
	for i := uint64(0); i < blocksToSend; {
		data, offset, nd, err := hnd.ufsr.ReadBlock(context.TODO())
		if err != nil {
			return err
		}

		log.Infof("sending block for a deal: %s", nd.Cid())

		if offset != unixfs0.Offset {
			return xerrors.Errorf("ReadBlock on wrong offset: want %d, got %d", unixfs0.Offset, offset)
		}

		/*if uint64(len(data)) != deal.Unixfs0.Size { // TODO: Fix for internal nodes (and any other node too)
			writeErr(stream, xerrors.Errorf("ReadBlock data with wrong size: want %d, got %d", deal.Unixfs0.Size, len(data)))
			return
		}*/

		block := &Block{
			Prefix: nd.Cid().Prefix().Bytes(),
			Data:   nd.RawData(),
		}

		if err := cborutil.WriteCborRPC(hnd.stream, block); err != nil {
			return err
		}

		if len(data) > 0 { // don't count internal nodes
			hnd.at += uint64(len(data))
			i++
		}
	}

	return nil
}
