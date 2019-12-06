package seed

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/ipfs/go-datastore"
	"path/filepath"

	badger "github.com/ipfs/go-ds-badger"
	logging "github.com/ipfs/go-log"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
)

var log = logging.Logger("preseal")

type SealBatch struct {
	MinerAddr address.Address

	SectorSize uint64
	SectorIDs []uint64
	Preimage []byte
}

type SealBatchOut struct {
	MinerAddr address.Address

	SectorSize uint64
	SectorIDs []uint64
	CommDs [][]byte
	CommRs [][]byte
}

type GenesisMinerMeta struct {
	MinerAddr address.Address

	Key types.KeyInfo

	Owner  address.Address
	Worker address.Address

	Preimage []byte
	SectorSize uint64

	SectorIDs []uint64
}

func Prepare(sbroot string, maddr address.Address, ssize uint64, sectors int, partitions int, preimage []byte) (*GenesisMinerMeta, []SealBatch, error) {
	out := make([]SealBatch, partitions)

	for i := 0; i < partitions; i++ {
		out[i] = SealBatch{
			MinerAddr:  maddr,
			SectorSize: ssize,
			Preimage:   preimage,
		}
	}

	mds, err := badger.NewDatastore(filepath.Join(sbroot, "badger"), nil)
	if err != nil {
		return nil, nil, err
	}

	sb, err := sectorbuilder.New(mksbcfg(sbroot, maddr, ssize), mds)
	if err != nil {
		return nil, nil, err
	}

	sids := make([]uint64, sectors)

	for i := 0; i < sectors; i++ {
		sid, err := sb.AcquireSectorId()
		if err != nil {
			return nil, nil, err
		}

		sids[i] = sid

		partition := sid % uint64(partitions)

		out[partition].SectorIDs = append(out[partition].SectorIDs, sid)
	}

	if err := mds.Close(); err != nil {
		return nil, nil, xerrors.Errorf("closing datastore: %w", err)
	}

	minerKey, err := wallet.GenerateKey(types.KTBLS)
	if err != nil {
		return nil, nil, err
	}

	return &GenesisMinerMeta{
		MinerAddr:  maddr,
		Key:        minerKey.KeyInfo,
		Owner:      minerKey.Address,
		Worker:     minerKey.Address,
		Preimage:   preimage,
		SectorSize: ssize,
		SectorIDs:  sids,
	}, out, nil
}

func RunTask(sbroot string, task SealBatch) (*SealBatchOut, error) {
	sb, err := sectorbuilder.New(mksbcfg(sbroot, task.MinerAddr, task.SectorSize), datastore.NewMapDatastore())
	if err != nil {
		return nil, err
	}

	out := &SealBatchOut{
		MinerAddr:  task.MinerAddr,
		SectorSize: task.SectorSize,
		SectorIDs:  task.SectorIDs,
		CommDs:     make([][]byte, len(task.SectorIDs)),
		CommRs:     make([][]byte, len(task.SectorIDs)),
	}

	size := sectorbuilder.UserBytesForSectorSize(task.SectorSize)

	for i, sectorID := range task.SectorIDs {
		pi, err := sb.AddPiece(size, sectorID, zeroR, nil)
		if err != nil {
			return nil, err
		}

		fmt.Println("Piece info: ", pi)

		trand := sha256.Sum256(task.Preimage)
		ticket := sectorbuilder.SealTicket{
			TicketBytes: trand,
		}

		pco, err := sb.SealPreCommit(sectorID, ticket, []sectorbuilder.PublicPieceInfo{pi})
		if err != nil {
			return nil, xerrors.Errorf("precommit: %w", err)
		}

		if err := sb.TrimCache(sectorID); err != nil {
			return nil, xerrors.Errorf("trim cache: %w", err)
		}

		copy(out.CommDs[i], pco.CommD[:])
		copy(out.CommRs[i], pco.CommR[:])
	}

	return out, nil
}

func PreSeal(maddr address.Address, ssize uint64, sectors int, sbroot string, preimage []byte) (*genesis.GenesisMiner, error) {
	mds, err := badger.NewDatastore(filepath.Join(sbroot, "badger"), nil)
	if err != nil {
		return nil, err
	}

	if err := build.GetParams(ssize); err != nil {
		return nil, xerrors.Errorf("getting params: %w", err)
	}

	sb, err := sectorbuilder.New(mksbcfg(sbroot, maddr, ssize), mds)
	if err != nil {
		return nil, err
	}

	size := sectorbuilder.UserBytesForSectorSize(ssize)

	var sealedSectors []*genesis.PreSeal
	for i := 0; i < sectors; i++ {
		sid, err := sb.AcquireSectorId()
		if err != nil {
			return nil, err
		}

		pi, err := sb.AddPiece(size, sid, zeroR, nil)
		if err != nil {
			return nil, err
		}

		trand := sha256.Sum256(preimage)
		ticket := sectorbuilder.SealTicket{
			TicketBytes: trand,
		}

		fmt.Println("Piece info: ", pi)

		pco, err := sb.SealPreCommit(sid, ticket, []sectorbuilder.PublicPieceInfo{pi})
		if err != nil {
			return nil, xerrors.Errorf("commit: %w", err)
		}

		log.Warn("PreCommitOutput: ", sid, pco)
		sealedSectors = append(sealedSectors, &genesis.PreSeal{
			CommR:    pco.CommR,
			CommD:    pco.CommD,
			SectorID: sid,
		})
	}

	minerAddr, err := wallet.GenerateKey(types.KTBLS)
	if err != nil {
		return nil, err
	}

	miner := &genesis.GenesisMiner{
		Owner:  minerAddr.Address,
		Worker: minerAddr.Address,

		SectorSize: ssize,

		Sectors: sealedSectors,

		Key: minerAddr.KeyInfo,
	}

	if err := createDeals(miner, minerAddr, maddr, ssize); err != nil {
		return nil, xerrors.Errorf("creating deals: %w", err)
	}

	if err := mds.Close(); err != nil {
		return nil, xerrors.Errorf("closing datastore: %w", err)
	}

	return miner, nil
}

func createDeals(m *genesis.GenesisMiner, k *wallet.Key, maddr address.Address, ssize uint64) error {
	for _, sector := range m.Sectors {
		proposal := &actors.StorageDealProposal{
			PieceRef:             sector.CommD[:], // just one deal so this == CommP
			PieceSize:            sectorbuilder.UserBytesForSectorSize(ssize),
			PieceSerialization:   actors.SerializationUnixFSv0,
			Client:               k.Address,
			Provider:             maddr,
			ProposalExpiration:   9000, // TODO: allow setting
			Duration:             9000,
			StoragePricePerEpoch: types.NewInt(0),
			StorageCollateral:    types.NewInt(0),
			ProposerSignature:    nil,
		}

		if err := api.SignWith(context.TODO(), wallet.KeyWallet(k).Sign, k.Address, proposal); err != nil {
			return err
		}

		deal := &actors.StorageDeal{
			Proposal:         *proposal,
			CounterSignature: nil,
		}

		if err := api.SignWith(context.TODO(), wallet.KeyWallet(k).Sign, k.Address, deal); err != nil {
			return err
		}

		sector.Deal = *deal
	}

	return nil
}
