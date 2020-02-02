package sealing

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"sync"
)

type CachePiece struct {
	DealID uint64

	Size  uint64

	CommP0 []byte
	CommP1 []byte
	FileName string
}


var lock = &sync.Mutex{}

var path = ".lotusstorage/staging"

var firstSectorId = "s-XXX-0"

func (m *Sealing) StagedSectorPath(sectorID uint64) string {

	name := fmt.Sprintf("s-%s-%d", m.maddr, sectorID)

	dir,err:= homedir.Dir()
	if err == nil{
		return filepath.Join(dir,path, name)
	}

	return filepath.Join("root",path, name)
}

func (m *Sealing) cachePath() string {
	dir,err:= homedir.Dir()
	if err == nil{
		return filepath.Join(dir, path, "plege-sector-cache")
	}
	return filepath.Join("root", path, "plege-sector-cache")
}

func (m *Sealing) firstSectorFileName() string {
	dir,err:= homedir.Dir()
	if err == nil{
		return filepath.Join(dir, path, firstSectorId)
	}
	return filepath.Join("root", path, firstSectorId)
}

func (m *Sealing) loadCacheInfo() ([]CachePiece, error) {
	var fileName = m.cachePath()

	contents,err := ioutil.ReadFile(fileName)

	if err != nil {
		return nil, xerrors.New("failed to load the pledge sector cache file")
	}

	var cached []CachePiece
	decoder := gob.NewDecoder(bytes.NewReader(contents))
	err = decoder.Decode(&cached)

	return cached, err
}

func (m *Sealing) saveCacheInfo(sectorId uint64, input []Piece, deals []actors.StorageDealProposal) (error) {

	firstSectorFileName := m.firstSectorFileName()

	//err := os.Rename(m.StagedSectorPath(sectorId), firstSectorFileName)

	err := os.Rename(m.StagedSectorPath(sectorId), firstSectorFileName)

	if err != nil{
		log.Fatal(err)
		return err
	}

	err = os.Symlink(firstSectorFileName, m.StagedSectorPath(sectorId))
	if err != nil {
		log.Fatal(err)
		return err
	}

	sizes := len(input)

	out := make([]CachePiece, sizes)

	var i int
	for i = 0; i < sizes; i++ {
		out[i] = CachePiece{
			DealID: input[i].DealID,
			Size:   input[i].Size,
			CommP0:  deals[i].PieceRef,
			CommP1:  input[i].CommP[:],
			FileName: firstSectorFileName,
		}
	}

	var result bytes.Buffer
	encoder := gob.NewEncoder(&result)
	err = encoder.Encode(out)
	if err != nil{
		return err
	}
	userBytes := result.Bytes()

	var fileName = m.cachePath()
	err = ioutil.WriteFile(fileName, userBytes,0666)
	if err!= nil {
		return xerrors.New("failed to save the pledge sector cache file")
	}

	return err
}

func (m *Sealing) repledgeSector(ctx context.Context, sectorID uint64, existingPieceSizes []uint64,  sizes ...uint64) ([]Piece, error) {

	pieces, err:= m.loadCacheInfo();

	if err != nil{
		return nil, err
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
