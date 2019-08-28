package retrieval

import (
	"context"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-msgio"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/lib/cborrpc"
	"github.com/filecoin-project/go-lotus/storage/sectorblocks"
	pb "github.com/ipfs/go-bitswap/message/pb"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/libp2p/go-libp2p-core/network"
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
	m *Miner
	stream network.Stream

	ufsr sectorblocks.UnixfsReader
	open cid.Cid
	at uint64
	size uint64
}

func (m *Miner) HandleDealStream(stream network.Stream) { // TODO: should we block in stream handlers
	defer stream.Close()

	hnd := &handlerDeal{
		m: m,

		stream: stream,
	}

	for {
		err := hnd.handleNext() // TODO: 'more' bool
		if err != nil {
			writeErr(stream, err)
			return
		}
	}

}

func (hnd *handlerDeal) handleNext() error {
	var deal Deal
	if err := cborrpc.ReadCborRPC(hnd.stream, &deal); err != nil {
		return err
	}

	if deal.Unixfs0 == nil {
		return xerrors.New("unknown deal type")
	}

	// TODO: Verify payment, check how much we can send based on that
	//  Or reject (possibly returning the payment to retain reputation with the client)

	if hnd.open != deal.Unixfs0.Root || hnd.at != deal.Unixfs0.Offset {
		log.Infof("opening file for sending (open '%s') (@%d, want %d)", hnd.open, hnd.at, deal.Unixfs0.Offset)
		if err := hnd.openFile(deal); err != nil {
			return err
		}
	}

	if deal.Unixfs0.Offset+deal.Unixfs0.Size > hnd.size {
		return xerrors.Errorf("tried to read too much %d+%d > %d", deal.Unixfs0.Offset, deal.Unixfs0.Size, hnd.size)
	}

	return hnd.accept(deal)
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

	buf := make([]byte, network.MessageSizeMax)
	msgw := msgio.NewVarintWriter(hnd.stream)

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

		block := pb.Message_Block{
			Prefix: nd.Cid().Prefix().Bytes(),
			Data:   nd.RawData(),
		}

		n, err := block.MarshalTo(buf)
		if err != nil {
			return err
		}

		if err := msgw.WriteMsg(buf[:n]); err != nil {
			log.Error(err)
			return err
		}

		if len(data) > 0 { // don't count internal nodes
			hnd.at += uint64(len(data))
			i++
		}
	}

	return nil
}
