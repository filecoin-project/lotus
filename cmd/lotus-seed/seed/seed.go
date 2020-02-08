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

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	badger "github.com/ipfs/go-ds-badger2"
	logging "github.com/ipfs/go-log/v2"
	"github.com/multiformats/go-multihash"
	"golang.org/x/xerrors"

	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/genesis"
)

var log = logging.Logger("preseal")

func PreSeal(maddr address.Address, ssize abi.SectorSize, offset uint64, sectors int, sbroot string, preimage []byte) (*genesis.GenesisMiner, error) {
	cfg := &sectorbuilder.Config{
		Miner:          maddr,
		SectorSize:     ssize,
		FallbackLastID: offset,
		Paths:          sectorbuilder.SimplePath(sbroot),
		WorkerThreads:  2,
	}

	if err := os.MkdirAll(sbroot, 0775); err != nil {
		return nil, err
	}

	mds, err := badger.NewDatastore(filepath.Join(sbroot, "badger"), nil)
	if err != nil {
		return nil, err
	}

	sb, err := sectorbuilder.New(cfg, namespace.Wrap(mds, datastore.NewKey("/sectorbuilder")))
	if err != nil {
		return nil, err
	}

	var sealedSectors []*genesis.PreSeal
	for i := 0; i < sectors; i++ {
		sid, err := sb.AcquireSectorId()
		if err != nil {
			return nil, err
		}

		pi, err := sb.AddPiece(context.TODO(), abi.PaddedPieceSize(ssize).Unpadded(), sid, rand.Reader, nil)
		if err != nil {
			return nil, err
		}

		trand := sha256.Sum256(preimage)
		ticket := sectorbuilder.SealTicket{
			TicketBytes: trand,
		}

		fmt.Printf("sector-id: %d, piece info: %v", sid, pi)

		pco, err := sb.SealPreCommit(context.TODO(), sid, ticket, []sectorbuilder.PublicPieceInfo{pi})
		if err != nil {
			return nil, xerrors.Errorf("commit: %w", err)
		}

		if err := sb.TrimCache(context.TODO(), sid); err != nil {
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

	if err := createDeals(miner, minerAddr, maddr, abi.SectorSize(ssize)); err != nil {
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

func commDCID(commd []byte) cid.Cid {
	d, err := cid.Prefix{
		Version:  1,
		Codec:    cid.Raw,
		MhType:   multihash.IDENTITY,
		MhLength: len(commd),
	}.Sum(commd)
	if err != nil {
		panic(err)
	}
	return d
}

func createDeals(m *genesis.GenesisMiner, k *wallet.Key, maddr address.Address, ssize abi.SectorSize) error {
	for _, sector := range m.Sectors {
		pref := make([]byte, len(sector.CommD))
		copy(pref, sector.CommD[:])
		proposal := &market.DealProposal{
			PieceCID:             commDCID(pref), // just one deal so this == CommP
			PieceSize:            abi.PaddedPieceSize(ssize),
			Client:               k.Address,
			Provider:             maddr,
			StartEpoch:           1, // TODO: allow setting
			EndEpoch:             9001,
			StoragePricePerEpoch: big.Zero(),
			ProviderCollateral:   big.Zero(),
			ClientCollateral:     big.Zero(),
		}

		sector.Deal = *proposal
	}

	return nil
}
