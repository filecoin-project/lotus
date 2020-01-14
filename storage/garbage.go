package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"os"

	sectorbuilder "github.com/xjrwfilecoin/go-sectorbuilder"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

func (m *Miner) pledgeSector(ctx context.Context, sectorID uint64, existingPieceSizes []uint64, sizes ...uint64) ([]Piece, error) {
	if len(sizes) == 0 {
		return nil, nil
	}

	deals := make([]actors.StorageDealProposal, len(sizes))
	for i, size := range sizes {
		release := m.sb.RateLimit()

		dataFileName := fmt.Sprintf("/tmp/file%d.dat", sizes[0])
		file1, err := os.Open(dataFileName)
		var commP [sectorbuilder.CommLen]byte

		commP1, err := m.presealFile(size) //sectorbuilder.GeneratePieceCommitment(io.LimitReader(rand.New(rand.NewSource(42)), int64(size)), size)
		copy(commP[:],commP1[:sectorbuilder.CommLen])
		release()


		if err != nil {
			commP, err = sectorbuilder.GeneratePieceCommitment(io.LimitReader(rand.New(rand.NewSource(42)), int64(size)), size)
			m.presealFile(size)	

			if err != nil {
				return nil, err
			}	
		}else{
			commP, err = sectorbuilder.GeneratePieceCommitment(file1,size)


			if err != nil {
				return nil, err
			}
		}

		release()
		file1.Close()
		sdp := actors.StorageDealProposal{
			PieceRef:             commP[:],
			PieceSize:            size,
			Client:               m.worker,
			Provider:             m.maddr,
			ProposalExpiration:   math.MaxUint64,
			Duration:             math.MaxUint64 / 2, // /2 because overflows
			StoragePricePerEpoch: types.NewInt(0),
			StorageCollateral:    types.NewInt(0),
			ProposerSignature:    nil,
		}

		if err := api.SignWith(ctx, m.api.WalletSign, m.worker, &sdp); err != nil {
			return nil, xerrors.Errorf("signing storage deal failed: ", err)
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
	dataFileName := fmt.Sprintf("/tmp/file%d.dat", sizes[0])
	file1, err := os.Open(dataFileName)
	defer file1.Close()
	if err != nil {
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
	} else {
		for i, size := range sizes {
			ppi, err := m.sb.AddPiece(size, sectorID, file1, existingPieceSizes)
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
	}

	return out, nil
}

func (m *Miner) presealFile(size uint64) (commP []byte, err error) {
	//检查文件是否存在
	dataFileName := fmt.Sprintf("/tmp/file%d.dat", size)
	commFileName := fmt.Sprintf("/tmp/comm%d.com", size)
	if _, err := os.Stat(dataFileName); os.IsNotExist(err) {
		// path/to/whatever does not exist
		randReader := io.LimitReader(rand.New(rand.NewSource(42)), int64(size))
		buffer := make([]byte, size)
		randReader.Read(buffer)
		ioutil.WriteFile(dataFileName, buffer, os.ModePerm)
		//return [ffi.CommitmentBytesLen]byte{}, nil
		file1, err := os.Open(dataFileName)
		defer file1.Close()
		if err != nil {
			log.Errorf("create temp file failed %v,error is: %v", dataFileName, err)
			return []byte{}, err
		}
		commP3, err3 := sectorbuilder.GeneratePieceCommitment(file1, uint64(size))

		if err3 != nil {
			log.Errorf("GeneratePieceCommitment failed %v,error is: %v", commFileName, err3)
			return []byte{}, err3
		}
		err3 = ioutil.WriteFile(commFileName, commP3[:], os.ModePerm)

		if err3 != nil {
			log.Errorf("save commentment to file %v failed,error is: %v", commFileName, err3)
			return []byte{}, err3
		}

		return commP3[:], nil
	}

	if _, err := os.Stat(commFileName); os.IsNotExist(err) {
		file1, err := os.Open(dataFileName)
		defer file1.Close()
		if err != nil {
			log.Errorf("create temp file failed %v,error is: %v", dataFileName, err)
			return []byte{}, err
		}
		// path/to/whatever does not exist
		commP3, err3 := sectorbuilder.GeneratePieceCommitment(file1, uint64(size))
		if err3 != nil {
			log.Errorf("GeneratePieceCommitment failed %v,error is: %v", dataFileName, err3)
			return []byte{}, err3
		}
		err3 = ioutil.WriteFile(commFileName, commP3[:], os.ModePerm)

		if err3 != nil {
			log.Errorf("save commentment to file %v failed,error is: %v", commFileName, err3)
			return []byte{}, err3
		}
		return commP3[:], nil
	}
	bytesval, err := ioutil.ReadFile(commFileName)
	if err != nil {
		log.Errorf("fail to read commitment file %v failed,error is: %v", commFileName, err)
		return []byte{}, err
	}

	return bytesval, nil
}
func (m *Miner) PledgeSector() error {
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
