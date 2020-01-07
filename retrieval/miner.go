package retrieval

import (
	"context"
	"io"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/libp2p/go-libp2p-core/network"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
)

type RetrMinerApi interface {
	PaychVoucherAdd(context.Context, address.Address, *types.SignedVoucher, []byte, types.BigInt) (types.BigInt, error)
}

type Miner struct {
	sectorBlocks *sectorblocks.SectorBlocks
	full         RetrMinerApi

	pricePerByte types.BigInt
	// TODO: Unseal price
}

func NewMiner(sblks *sectorblocks.SectorBlocks, full api.FullNode) *Miner {
	return &Miner{
		sectorBlocks: sblks,
		full:         full,

		pricePerByte: types.NewInt(2), // TODO: allow setting
	}
}

func writeErr(stream network.Stream, err error) {
	log.Errorf("Retrieval deal error: %+v", err)
	_ = cborutil.WriteCborRPC(stream, &DealResponse{
		Status:  Error,
		Message: err.Error(),
	})
}

func (m *Miner) HandleQueryStream(stream network.Stream) {
	defer stream.Close()

	var query Query
	if err := cborutil.ReadCborRPC(stream, &query); err != nil {
		writeErr(stream, err)
		return
	}

	size, err := m.sectorBlocks.GetSize(query.Piece)
	if err != nil && err != sectorblocks.ErrNotFound {
		log.Errorf("Retrieval query: GetRefs: %s", err)
		return
	}

	answer := &QueryResponse{
		Status: Unavailable,
	}
	if err == nil {
		answer.Status = Available

		// TODO: get price, look for already unsealed ref to reduce work
		answer.MinPrice = types.BigMul(types.NewInt(uint64(size)), m.pricePerByte)
		answer.Size = uint64(size) // TODO: verify on intermediate
	}

	if err := cborutil.WriteCborRPC(stream, answer); err != nil {
		log.Errorf("Retrieval query: WriteCborRPC: %s", err)
		return
	}
}

type handlerDeal struct {
	m      *Miner
	stream network.Stream

	ufsr sectorblocks.UnixfsReader
	open cid.Cid
	at   uint64
	size uint64
}

func (m *Miner) HandleDealStream(stream network.Stream) {
	defer stream.Close()

	hnd := &handlerDeal{
		m: m,

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
	var deal DealProposal
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

	expPayment := types.BigMul(hnd.m.pricePerByte, types.NewInt(deal.Params.Unixfs0.Size))
	if _, err := hnd.m.full.PaychVoucherAdd(context.TODO(), deal.Payment.Channel, deal.Payment.Vouchers[0], nil, expPayment); err != nil {
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

func (hnd *handlerDeal) openFile(deal DealProposal) error {
	unixfs0 := deal.Params.Unixfs0

	if unixfs0.Offset != 0 {
		// TODO: Implement SeekBlock (like ReadBlock) in go-unixfs
		return xerrors.New("sending merkle proofs for nonzero offset not supported yet")
	}
	hnd.at = unixfs0.Offset

	bstore := hnd.m.sectorBlocks.SealedBlockstore(func() error {
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

func (hnd *handlerDeal) accept(deal DealProposal) error {
	unixfs0 := deal.Params.Unixfs0

	resp := &DealResponse{
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
