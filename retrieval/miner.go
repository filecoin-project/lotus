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

func (m *Miner) HandleQueryStream(stream network.Stream) {
	defer stream.Close()

	var query Query
	if err := cborrpc.ReadCborRPC(stream, &query); err != nil {
		log.Errorf("Retrieval query: ReadCborRPC: %s", err)
		return
	}

	refs, err := m.sectorBlocks.GetRefs(query.Piece)
	if err != nil {
		log.Errorf("Retrieval query: GetRefs: %s", err)
		return
	}

	answer := QueryResponse{
		Status: Unavailable,
	}
	if len(refs) > 0 {
		answer.Status = Available

		// TODO: get price, look for already unsealed ref to reduce work
		answer.MinPrice = types.BigMul(types.NewInt(uint64(refs[0].Size)), m.pricePerByte)
		answer.Size = uint64(refs[0].Size)                       // TODO: verify on intermediate
	}

	if err := cborrpc.WriteCborRPC(stream, answer); err != nil {
		log.Errorf("Retrieval query: WriteCborRPC: %s", err)
		return
	}
}

func writeErr(stream network.Stream, err error) {
	log.Errorf("Retrieval deal error: %s", err)
	_ = cborrpc.WriteCborRPC(stream, DealResponse{
		Status: Error,
		Message: err.Error(),
	})
}

func (m *Miner) HandleDealStream(stream network.Stream) { // TODO: should we block in stream handlers
	defer stream.Close()

	var ufsr sectorblocks.UnixfsReader
	var open cid.Cid
	var at uint64
	var size uint64

	for {
		var deal Deal
		if err := cborrpc.ReadCborRPC(stream, &deal); err != nil {
			return 
		}
		
		if deal.Unixfs0 == nil {
			writeErr(stream, xerrors.New("unknown deal type"))
			return
		}

		// TODO: Verify payment, check how much we can send based on that
		//  Or reject (possibly returning the payment to retain reputation with the client)

		bstore := m.sectorBlocks.SealedBlockstore(func() error {
			return nil // TODO: approve unsealing based on amount paid
		})

		if open != deal.Unixfs0.Root || at != deal.Unixfs0.Offset {
			if deal.Unixfs0.Offset != 0 {
				// TODO: Implement SeekBlock (like ReadBlock) in go-unixfs
				writeErr(stream, xerrors.New("sending merkle proofs for nonzero offset not supported yet"))
				return
			}
			at = deal.Unixfs0.Offset

			ds := merkledag.NewDAGService(blockservice.New(bstore, nil))
			rootNd, err := ds.Get(context.TODO(), deal.Unixfs0.Root)
			if err != nil {
				writeErr(stream, err)
				return
			}

			fsr, err := unixfile.NewUnixfsFile(context.TODO(), ds, rootNd)
			if err != nil {
				writeErr(stream, err)
				return
			}

			var ok bool
			ufsr, ok = fsr.(sectorblocks.UnixfsReader)
			if !ok {
				writeErr(stream, xerrors.Errorf("file %s didn't implement sectorblocks.UnixfsReader", deal.Unixfs0.Root))
				return
			}

			isize, err := ufsr.Size()
			if err != nil {
				writeErr(stream, err)
				return
			}
			size = uint64(isize)
		}

		if deal.Unixfs0.Offset + deal.Unixfs0.Size > size {
			writeErr(stream, xerrors.Errorf("tried to read too much %d+%d > %d", deal.Unixfs0.Offset, deal.Unixfs0.Size, size))
			return
		}

		resp := DealResponse{
			Status: Accepted,
		}
		if err := cborrpc.WriteCborRPC(stream, resp); err != nil {
			log.Errorf("Retrieval query: Write Accepted resp: %s", err)
			return
		}

		buf := make([]byte, network.MessageSizeMax)
		msgw := msgio.NewVarintWriter(stream)

		blocksToSend := (deal.Unixfs0.Size + build.UnixfsChunkSize - 1) / build.UnixfsChunkSize
		for i := uint64(0); i < blocksToSend; {
			data, offset, nd, err := ufsr.ReadBlock(context.TODO())
			if err != nil {
				writeErr(stream, err)
				return
			}

			log.Infof("sending block for a deal: %s", nd.Cid())

			if offset != deal.Unixfs0.Offset {
				writeErr(stream, xerrors.Errorf("ReadBlock on wrong offset: want %d, got %d", deal.Unixfs0.Offset, offset))
				return
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
				writeErr(stream, err)
				return
			}

			if err := msgw.WriteMsg(buf[:n]); err != nil {
				log.Error(err)
				return
			}

			if len(data) > 0 { // don't count internal nodes
				i++
			}
		}

		// TODO: set `at`

	}

}
