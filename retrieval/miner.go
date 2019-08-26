package retrieval

import (
	"github.com/libp2p/go-libp2p-core/network"

	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/lib/cborrpc"
	"github.com/filecoin-project/go-lotus/storage/sectorblocks"
)

type Miner struct {
	sectorBlocks *sectorblocks.SectorBlocks
}

func NewMiner(sblks *sectorblocks.SectorBlocks) *Miner {
	return &Miner{
		sectorBlocks: sblks,
	}
}

func (m *Miner) HandleStream(stream network.Stream) {
	defer stream.Close()

	var query RetQuery
	if err := cborrpc.ReadCborRPC(stream, &query); err != nil {
		log.Errorf("Retrieval query: ReadCborRPC: %s", err)
		return
	}

	refs, err := m.sectorBlocks.GetRefs(query.Piece)
	if err != nil {
		log.Errorf("Retrieval query: GetRefs: %s", err)
		return
	}

	answer := RetQueryResponse{
		Status: Unavailable,
	}
	if len(refs) > 0 {
		answer.Status = Available

		// TODO: get price, look for already unsealed ref to reduce work
		answer.MinPrice = types.NewInt(uint64(refs[0].Size)) // TODO: Get this from somewhere
		answer.Size = uint64(refs[0].Size)
	}

	if err := cborrpc.WriteCborRPC(stream, answer); err != nil {
		log.Errorf("Retrieval query: WriteCborRPC: %s", err)
		return
	}
}
