package sealing

import (
	"bytes"
	"context"
	"io"
	"math"
	"math/bits"
	"math/rand"
	"runtime"
	"sync"

	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

func (m *Sealing) fastPledgeCommitment(size uint64, parts uint64) (commP [sectorbuilder.CommLen]byte, err error) {
	parts = 1 << bits.Len64(parts) // round down to nearest power of 2

	piece := sectorbuilder.UserBytesForSectorSize((size/127 + size) / parts)
	out := make([]sectorbuilder.PublicPieceInfo, parts)
	var lk sync.Mutex

	var wg sync.WaitGroup
	wg.Add(int(parts))
	for i := uint64(0); i < parts; i++ {
		go func(i uint64) {
			defer wg.Done()

			commP, perr := sectorbuilder.GeneratePieceCommitment(io.LimitReader(rand.New(rand.NewSource(42+int64(i))), int64(piece)), piece)

			lk.Lock()
			if perr != nil {
				err = multierror.Append(err, perr)
			}
			out[i] = sectorbuilder.PublicPieceInfo{
				Size:  piece,
				CommP: commP,
			}
			lk.Unlock()
		}(i)
	}
	wg.Wait()

	if err != nil {
		return [32]byte{}, err
	}

	return sectorbuilder.GenerateDataCommitment(m.sb.SectorSize(), out)
}

func (m *Sealing) pledgeReader(size uint64, parts uint64) io.Reader {
	piece := sectorbuilder.UserBytesForSectorSize((size/127 + size) / parts)

	readers := make([]io.Reader, parts)
	for i := range readers {
		readers[i] = io.LimitReader(rand.New(rand.NewSource(42+int64(i))), int64(piece))
	}

	return io.MultiReader(readers...)
}

func (m *Sealing) pledgeSector(ctx context.Context, sectorID uint64, existingPieceSizes []uint64, sizes ...uint64) ([]Piece, error) {
	if len(sizes) == 0 {
		return nil, nil
	}

	deals := make([]actors.StorageDealProposal, len(sizes))
	for i, size := range sizes {
		commP, err := m.fastPledgeCommitment(size, uint64(runtime.NumCPU()))
		if err != nil {
			return nil, err
		}

		sdp := actors.StorageDealProposal{
			PieceRef:             commP[:],
			PieceSize:            size,
			Client:               m.worker,
			Provider:             m.maddr,
			ProposalExpiration:   math.MaxUint64,
			Duration:             math.MaxUint64 / 2, // /2 because overflows
			StoragePricePerEpoch: types.NewInt(0),
			StorageCollateral:    types.NewInt(0),
			ProposerSignature:    nil, // nil because self dealing
		}

		deals[i] = sdp
	}

	params, aerr := actors.SerializeParams(&actors.PublishStorageDealsParams{
		Deals: deals,
	})
	if aerr != nil {
		return nil, xerrors.Errorf("serializing PublishStorageDeals params failed: ", aerr)
	}

	smsg, err := m.api.MpoolPushMessage(ctx, &types.Message{
		To:       actors.StorageMarketAddress,
		From:     m.worker,
		Value:    types.NewInt(0),
		GasPrice: types.NewInt(0),
		GasLimit: types.NewInt(1000000),
		Method:   actors.SMAMethods.PublishStorageDeals,
		Params:   params,
	})
	if err != nil {
		return nil, err
	}
	r, err := m.api.StateWaitMsg(ctx, smsg.Cid())
	if err != nil {
		return nil, err
	}
	if r.Receipt.ExitCode != 0 {
		log.Error(xerrors.Errorf("publishing deal failed: exit %d", r.Receipt.ExitCode))
	}
	var resp actors.PublishStorageDealResponse
	if err := resp.UnmarshalCBOR(bytes.NewReader(r.Receipt.Return)); err != nil {
		return nil, err
	}
	if len(resp.DealIDs) != len(sizes) {
		return nil, xerrors.New("got unexpected number of DealIDs from PublishStorageDeals")
	}

	out := make([]Piece, len(sizes))

	for i, size := range sizes {
		ppi, err := m.sb.AddPiece(size, sectorID, m.pledgeReader(size, uint64(runtime.NumCPU())), existingPieceSizes)
		if err != nil {
			return nil, err
		}

		existingPieceSizes = append(existingPieceSizes, size)

		out[i] = Piece{
			DealID: resp.DealIDs[i],
			Size:   ppi.Size,
			CommP:  ppi.CommP[:],
		}
	}

	return out, nil
}

func (m *Sealing) PledgeSector() error {
	go func() {
		ctx := context.TODO() // we can't use the context from command which invokes
		// this, as we run everything here async, and it's cancelled when the
		// command exits

		size := sectorbuilder.UserBytesForSectorSize(m.sb.SectorSize())

		sid, err := m.sb.AcquireSectorId()
		if err != nil {
			log.Errorf("%+v", err)
			return
		}

		pieces, err := m.pledgeSector(ctx, sid, []uint64{}, size)
		if err != nil {
			log.Errorf("%+v", err)
			return
		}

		if err := m.newSector(context.TODO(), sid, pieces[0].DealID, pieces[0].ppi()); err != nil {
			log.Errorf("%+v", err)
			return
		}
	}()
	return nil
}
