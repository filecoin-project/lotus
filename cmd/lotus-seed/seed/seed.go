package seed

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
	badger "github.com/ipfs/go-ds-badger"
	logging "github.com/ipfs/go-log"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/genesis"
)

var log = logging.Logger("preseal")

func PreSeal(maddr address.Address, ssize uint64, offset uint64, sectors int, sbroot string, preimage []byte) (*genesis.GenesisMiner, error) {
	cfg := &sectorbuilder.Config{
		Miner:          maddr,
		SectorSize:     ssize,
		FallbackLastID: offset,
		Dir:            sbroot,
		WorkerThreads:  2,
	}

	if err := os.MkdirAll(sbroot, 0775); err != nil {
		return nil, err
	}

	mds, err := badger.NewDatastore(filepath.Join(sbroot, "badger"), nil)
	if err != nil {
		return nil, err
	}

	sb, err := sectorbuilder.New(cfg, mds)
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

		pi, err := sb.AddPiece(size, sid, rand.Reader, nil)
		if err != nil {
			return nil, err
		}

		trand := sha256.Sum256(preimage)
		ticket := sectorbuilder.SealTicket{
			TicketBytes: trand,
		}

		fmt.Printf("sector-id: %d, piece info: %v", sid, pi)

		pco, err := sb.SealPreCommit(sid, ticket, []sectorbuilder.PublicPieceInfo{pi})
		if err != nil {
			return nil, xerrors.Errorf("commit: %w", err)
		}

		if err := sb.TrimCache(sid); err != nil {
			return nil, xerrors.Errorf("trim cache: %w", err)
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
			Client:               k.Address,
			Provider:             maddr,
			ProposalExpiration:   9000, // TODO: allow setting
			Duration:             9000,
			StoragePricePerEpoch: types.NewInt(0),
			StorageCollateral:    types.NewInt(0),
			ProposerSignature:    nil,
		}

		// TODO: pretty sure we don't even need to sign this
		if err := api.SignWith(context.TODO(), wallet.KeyWallet(k).Sign, k.Address, proposal); err != nil {
			return err
		}

		sector.Deal = *proposal
	}

	return nil
}
