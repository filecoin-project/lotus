package seed

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"

	badger "github.com/ipfs/go-ds-badger"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
)

func PreSeal(maddr address.Address, ssize uint64, sectors uint64, sbroot string, preimage []byte) error {
	cfg := &sectorbuilder.Config{
		Miner:         maddr,
		SectorSize:    ssize,
		CacheDir:      filepath.Join(sbroot, "cache"),
		SealedDir:     filepath.Join(sbroot, "sealed"),
		StagedDir:     filepath.Join(sbroot, "staging"),
		MetadataDir:   filepath.Join(sbroot, "meta"),
		WorkerThreads: 2,
	}

	for _, d := range []string{cfg.CacheDir, cfg.SealedDir, cfg.StagedDir, cfg.MetadataDir} {
		if err := os.MkdirAll(d, 0775); err != nil {
			return err
		}
	}

	mds, err := badger.NewDatastore(filepath.Join(sbroot, "badger"), nil)
	if err != nil {
		return err
	}

	if err := build.GetParams(true, false); err != nil {
		return xerrors.Errorf("getting params: %w", err)
	}

	sb, err := sectorbuilder.New(cfg, mds)
	if err != nil {
		return err
	}

	r := rand.New(rand.NewSource(101))
	size := sectorbuilder.UserBytesForSectorSize(ssize)

	var sealedSectors []genesis.PreSeal
	for i := uint64(1); i <= sectors; i++ {
		sid, err := sb.AcquireSectorId()
		if err != nil {
			return err
		}

		pi, err := sb.AddPiece(size, sid, r, nil)
		if err != nil {
			return err
		}

		trand := sha256.Sum256(preimage)
		ticket := sectorbuilder.SealTicket{
			TicketBytes: trand,
		}

		fmt.Println("Piece info: ", pi)

		pco, err := sb.SealPreCommit(sid, ticket, []sectorbuilder.PublicPieceInfo{pi})
		if err != nil {
			return xerrors.Errorf("commit: %w", err)
		}

		sealedSectors = append(sealedSectors, genesis.PreSeal{
			CommR:    pco.CommR,
			CommD:    pco.CommD,
			SectorID: sid,
		})
	}

	output := map[string]genesis.GenesisMiner{
		maddr.String(): genesis.GenesisMiner{
			Sectors: sealedSectors,
		},
	}

	out, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(filepath.Join(sbroot, "pre-seal-"+maddr.String()+".json"), out, 0664); err != nil {
		return err
	}

	if err := mds.Close(); err != nil {
		return xerrors.Errorf("closing datastore: %w", err)
	}

	return nil
}
