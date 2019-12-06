package seed

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
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

func PreSeal(maddr address.Address, ssize uint64, sectors int, sbroot string, preimage []byte) (*genesis.GenesisMiner, error) {
	cfg := &sectorbuilder.Config{
		Miner:         maddr,
		SectorSize:    ssize,
		CacheDir:      filepath.Join(sbroot, "cache"),
		SealedDir:     filepath.Join(sbroot, "sealed"),
		StagedDir:     filepath.Join(sbroot, "staging"),
		UnsealedDir:   filepath.Join(sbroot, "unsealed"),
		WorkerThreads: 2,
	}

	for _, d := range []string{cfg.CacheDir, cfg.SealedDir, cfg.StagedDir, cfg.UnsealedDir} {
		if err := os.MkdirAll(d, 0775); err != nil {
			return nil, err
		}
	}

	mds, err := badger.NewDatastore(filepath.Join(sbroot, "badger"), nil)
	if err != nil {
		return nil, err
	}

	if err := build.GetParams(ssize); err != nil {
		return nil, xerrors.Errorf("getting params: %w", err)
	}

	sb, err := sectorbuilder.New(cfg, mds)
	if err != nil {
		return nil, err
	}

	r := rand.New(rand.NewSource(101))
	size := sectorbuilder.UserBytesForSectorSize(ssize)

	var sealedSectors []*genesis.PreSeal
	for i := 0; i < sectors; i++ {
		sid, err := sb.AcquireSectorId()
		if err != nil {
			return nil, err
		}

		pi, err := sb.AddPiece(size, sid, r, nil)
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

func WriteGenesisMiner(maddr address.Address, sbroot string, gm *genesis.GenesisMiner) error {
	output := map[string]genesis.GenesisMiner{
		maddr.String(): *gm,
	}

	out, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(filepath.Join(sbroot, "pre-seal-"+maddr.String()+".json"), out, 0664); err != nil {
		return err
	}

	return nil
}

func createDeals(m *genesis.GenesisMiner, k *wallet.Key, maddr address.Address, ssize uint64) error {
	for _, sector := range m.Sectors {
		pref := make([]byte, len(sector.CommD))
		copy(pref, sector.CommD[:])
		proposal := &actors.StorageDealProposal{
			PieceRef:             pref, // just one deal so this == CommP
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
