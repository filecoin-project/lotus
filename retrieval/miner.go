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

	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/lib/cborrpc"
	"github.com/filecoin-project/go-lotus/storage/sectorblocks"
)

type Miner struct {
	sectorBlocks *sectorblocks.SectorBlocks

	pricePerByte types.BigInt
	// TODO: Unseal price
}

func NewMiner(sblks *sectorblocks.SectorBlocks) *Miner {
	return &Miner{
		sectorBlocks: sblks,
		pricePerByte: types.NewInt(2), // TODO: allow setting
	}
}

func writeErr(stream network.Stream, err error) {
	log.Errorf("Retrieval deal error: %s", err)
	_ = cborrpc.WriteCborRPC(stream, DealResponse{
		Status:  Error,
		Message: err.Error(),
	})
}

func (m *Miner) HandleQueryStream(stream network.Stream) {
	defer stream.Close()

	var query Query
	if err := cborrpc.ReadCborRPC(stream, &query); err != nil {
		writeErr(stream, err)
		return
	}

	size, err := m.sectorBlocks.GetSize(query.Piece)
	if err != nil && err != sectorblocks.ErrNotFound {
		log.Errorf("Retrieval query: GetRefs: %s", err)
		return
	}

	answer := QueryResponse{
		Status: Unavailable,
	}
	if err == nil {
		answer.Status = Available

		// TODO: get price, look for already unsealed ref to reduce work
		answer.MinPrice = types.BigMul(types.NewInt(uint64(size)), m.pricePerByte)
		answer.Size = uint64(size) // TODO: verify on intermediate
	}

	if err := cborrpc.WriteCborRPC(stream, answer); err != nil {
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
	var deal Deal
	if err := cborrpc.ReadCborRPC(hnd.stream, &deal); err != nil {
		if err == io.EOF { // client sent all deals
			err = nil
		}
		return false, err
	}

	if deal.Unixfs0 == nil {
		return false, xerrors.New("unknown deal type")
	}

	// TODO: Verify payment, check how much we can send based on that
	//  Or reject (possibly returning the payment to retain reputation with the client)

	if hnd.open != deal.Unixfs0.Root || hnd.at != deal.Unixfs0.Offset {
		log.Infof("opening file for sending (open '%s') (@%d, want %d)", hnd.open, hnd.at, deal.Unixfs0.Offset)
		if err := hnd.openFile(deal); err != nil {
			return false, err
		}
	}

	if deal.Unixfs0.Offset+deal.Unixfs0.Size > hnd.size {
		return false, xerrors.Errorf("tried to read too much %d+%d > %d", deal.Unixfs0.Offset, deal.Unixfs0.Size, hnd.size)
	}

	err := hnd.accept(deal)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (hnd *handlerDeal) openFile(deal Deal) error {
	if deal.Unixfs0.Offset != 0 {
		// TODO: Implement SeekBlock (like ReadBlock) in go-unixfs
		return xerrors.New("sending merkle proofs for nonzero offset not supported yet")
	}
	hnd.at = deal.Unixfs0.Offset

	bstore := hnd.m.sectorBlocks.SealedBlockstore(func() error {
		return nil // TODO: approve unsealing based on amount paid
	})

	ds := merkledag.NewDAGService(blockservice.New(bstore, nil))
	rootNd, err := ds.Get(context.TODO(), deal.Unixfs0.Root)
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
		return xerrors.Errorf("file %s didn't implement sectorblocks.UnixfsReader", deal.Unixfs0.Root)
	}

	isize, err := hnd.ufsr.Size()
	if err != nil {
		return err
	}
	hnd.size = uint64(isize)

	hnd.open = deal.Unixfs0.Root

	return nil
}

func (hnd *handlerDeal) accept(deal Deal) error {
	resp := DealResponse{
		Status: Accepted,
	}
	if err := cborrpc.WriteCborRPC(hnd.stream, resp); err != nil {
		log.Errorf("Retrieval query: Write Accepted resp: %s", err)
		return err
	}

	blocksToSend := (deal.Unixfs0.Size + build.UnixfsChunkSize - 1) / build.UnixfsChunkSize
	for i := uint64(0); i < blocksToSend; {
		data, offset, nd, err := hnd.ufsr.ReadBlock(context.TODO())
		if err != nil {
			return err
		}

		log.Infof("sending block for a deal: %s", nd.Cid())

		if offset != deal.Unixfs0.Offset {
			return xerrors.Errorf("ReadBlock on wrong offset: want %d, got %d", deal.Unixfs0.Offset, offset)
		}

		/*if uint64(len(data)) != deal.Unixfs0.Size { // TODO: Fix for internal nodes (and any other node too)
			writeErr(stream, xerrors.Errorf("ReadBlock data with wrong size: want %d, got %d", deal.Unixfs0.Size, len(data)))
			return
		}*/

		block := Block{
			Prefix: nd.Cid().Prefix().Bytes(),
			Data:   nd.RawData(),
		}

		if err := cborrpc.WriteCborRPC(hnd.stream, block); err != nil {
			return err
		}

		if len(data) > 0 { // don't count internal nodes
			hnd.at += uint64(len(data))
			i++
		}
	}

	return nil
}
