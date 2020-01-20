package sealing

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sync"

	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)


type CachePiece struct {
	DealID uint64

	Size  uint64

	CommP0 []byte
	CommP1 []byte
	FileName string
}


var lock = &sync.Mutex{}

func (m *Sealing) StagedSectorPath(sectorID uint64) string {

	name := fmt.Sprintf("s-%s-%d", m.maddr, sectorID)

	return filepath.Join("~/.lotusstorage/staging", name)
}


func (m *Sealing) isFileExist(path string) (bool, error) {
	fileInfo, err := os.Stat(path)

	if os.IsNotExist(err) {
		return false, nil
	}


	if fileInfo.Size() == 0 {
		return false, nil
	}
	if err == nil {
		return true, nil
	}
	return false, err
}

func (m *Sealing) loadCacheInfo() ([]CachePiece, error) {
	var fileName = "~/.lotusstorage/plege-sector-cache";

	contents,err := ioutil.ReadFile(fileName);

	if err != nil {
		return nil, xerrors.New("failed to load the pledge sector cache file")
	}


	var cached []CachePiece
	decoder := gob.NewDecoder(bytes.NewReader(contents))
	err = decoder.Decode(&cached)

	return cached, err
}

func (m *Sealing) saveCacheInfo(sectorId uint64, input []Piece, deals []actors.StorageDealProposal) (error) {
	var fileName = "~/.lotusstorage/plege-sector-cache";

	sizes := len(input)

    out := make([]CachePiece, sizes)

	var i int
	for i = 0; i < sizes; i++ {
		out[i] = CachePiece{
			DealID: input[i].DealID,
			Size:   input[i].Size,
			CommP0:  deals[i].PieceRef,
			CommP1:  input[i].CommP[:],
            FileName: m.StagedSectorPath(sectorId),
		}
	}

	var result bytes.Buffer
	encoder := gob.NewEncoder(&result)
	encoder.Encode(out)
	userBytes := result.Bytes()

	err := ioutil.WriteFile(fileName, userBytes,0666)
	if err!= nil {
		return xerrors.New("failed to save the pledge sector cache file")
	}

	return err
}

func (m *Sealing) repledgeSector(ctx context.Context, sectorID uint64, existingPieceSizes []uint64, pieces []CachePiece, sizes ...uint64) ([]Piece, error) {
	if len(sizes) == 0 {
		return nil, nil
	}

	deals := make([]actors.StorageDealProposal, len(sizes))

	out := make([]Piece, len(sizes))

	for i, size := range sizes {

		sdp := actors.StorageDealProposal{
			PieceRef:             pieces[i].CommP0[:],
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

	for i, size := range sizes {

		existingPieceSizes = append(existingPieceSizes, size)

		out[i] = Piece{
			DealID: resp.DealIDs[i],
			Size:   pieces[i].Size,
			CommP:  pieces[i].CommP1[:],
		}

		err = os.Symlink(pieces[i].FileName, m.StagedSectorPath(sectorID))
		if err != nil {
			log.Fatal(err)
		}
	}

	return out, nil
}


func (m *Sealing) pledgeSector(ctx context.Context, sectorID uint64, existingPieceSizes []uint64, sizes ...uint64) ([]Piece, error) {
	if len(sizes) == 0 {
		return nil, nil
	}

	lock.Lock()

	cached, err:= m.loadCacheInfo();

	if err == nil {
		out, err := m.repledgeSector(ctx,sectorID,existingPieceSizes,cached, uint64(len(sizes)))

		lock.Unlock()

		return out, err
	}

	lock.Unlock()

	return m.firstPledgeSector(ctx,sectorID, existingPieceSizes, uint64(len(sizes)))
}

func (m *Sealing) firstPledgeSector(ctx context.Context, sectorID uint64, existingPieceSizes []uint64, sizes ...uint64) ([]Piece, error) {
	deals := make([]actors.StorageDealProposal, len(sizes))
	for i, size := range sizes {
		release := m.sb.RateLimit()
		commP, err := sectorbuilder.GeneratePieceCommitment(io.LimitReader(rand.New(rand.NewSource(42)), int64(size)), size)
		release()

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
		ppi, err := m.sb.AddPiece(size, sectorID, io.LimitReader(rand.New(rand.NewSource(42)), int64(size)), existingPieceSizes)
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

	m.saveCacheInfo(sectorID, out,deals)

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