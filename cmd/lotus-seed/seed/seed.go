package seed

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/go-sectorbuilder/fs"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/crypto"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/node/config"
)

var log = logging.Logger("preseal")

func PreSeal(maddr address.Address, pt abi.RegisteredProof, offset abi.SectorNumber, sectors int, sbroot string, preimage []byte, key *types.KeyInfo) (*genesis.Miner, *types.KeyInfo, error) {
	ppt, err := pt.RegisteredPoStProof()
	if err != nil {
		return nil, nil, err
	}

	spt, err := pt.RegisteredSealProof()
	if err != nil {
		return nil, nil, err
	}

	cfg := &sectorbuilder.Config{
		Miner:         maddr,
		SealProofType: spt,
		PoStProofType: ppt,
	}

	if err := os.MkdirAll(sbroot, 0775); err != nil {
		return nil, nil, err
	}

	sbfs := &fs.Basic{
		Miner:  maddr,
		NextID: offset,
		Root:   sbroot,
	}

	sb, err := sectorbuilder.New(sbfs, cfg)
	if err != nil {
		return nil, nil, err
	}

	ssize, err := pt.SectorSize()
	if err != nil {
		return nil, nil, err
	}

	var sealedSectors []*genesis.PreSeal
	for i := 0; i < sectors; i++ {
		sid, err := sbfs.AcquireSectorNumber()
		if err != nil {
			return nil, nil, err
		}

		pi, err := sb.AddPiece(context.TODO(), abi.PaddedPieceSize(ssize).Unpadded(), sid, rand.Reader, nil)
		if err != nil {
			return nil, nil, err
		}

		trand := sha256.Sum256(preimage)
		ticket := abi.SealRandomness(trand[:])

		fmt.Printf("sector-id: %d, piece info: %v\n", sid, pi)

		in2, err := sb.SealPreCommit1(context.TODO(), sid, ticket, []abi.PieceInfo{pi})
		if err != nil {
			return nil, nil, xerrors.Errorf("commit: %w", err)
		}

		scid, ucid, err := sb.SealPreCommit2(context.TODO(), sid, in2)
		if err != nil {
			return nil, nil, xerrors.Errorf("commit: %w", err)
		}

		if err := sb.FinalizeSector(context.TODO(), sid); err != nil {
			return nil, nil, xerrors.Errorf("trim cache: %w", err)
		}

		log.Warn("PreCommitOutput: ", sid, scid, ucid)
		sealedSectors = append(sealedSectors, &genesis.PreSeal{
			CommR:     scid,
			CommD:     ucid,
			SectorID:  sid,
			ProofType: pt,
		})
	}

	var minerAddr *wallet.Key
	if key != nil {
		minerAddr, err = wallet.NewKey(*key)
		if err != nil {
			return nil, nil, err
		}
	} else {
		minerAddr, err = wallet.GenerateKey(crypto.SigTypeBLS)
		if err != nil {
			return nil, nil, err
		}
	}

	miner := &genesis.Miner{
		Owner:         minerAddr.Address,
		Worker:        minerAddr.Address,
		MarketBalance: big.Zero(),
		PowerBalance:  big.Zero(),
		SectorSize:    ssize,
		Sectors:       sealedSectors,
	}

	if err := createDeals(miner, minerAddr, maddr, ssize); err != nil {
		return nil, nil, xerrors.Errorf("creating deals: %w", err)
	}

	{
		b, err := json.MarshalIndent(&config.StorageMeta{
			ID:       uuid.New().String(),
			Weight:   0, // read-only
			CanSeal:  false,
			CanStore: false,
		}, "", "  ")
		if err != nil {
			return nil, nil, xerrors.Errorf("marshaling storage config: %w", err)
		}

		if err := ioutil.WriteFile(filepath.Join(sbroot, "storage.json"), b, 0644); err != nil {
			return nil, nil, xerrors.Errorf("persisting storage metadata (%s): %w", filepath.Join(sbroot, "storage.json"), err)
		}
	}

	return miner, &minerAddr.KeyInfo, nil
}

func WriteGenesisMiner(maddr address.Address, sbroot string, gm *genesis.Miner, key *types.KeyInfo) error {
	output := map[string]genesis.Miner{
		maddr.String(): *gm,
	}

	out, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}

	log.Infof("Writing preseal manifest to %s", filepath.Join(sbroot, "pre-seal-"+maddr.String()+".json"))

	if err := ioutil.WriteFile(filepath.Join(sbroot, "pre-seal-"+maddr.String()+".json"), out, 0664); err != nil {
		return err
	}

	if key != nil {
		b, err := json.Marshal(key)
		if err != nil {
			return err
		}

		// TODO: allow providing key
		if err := ioutil.WriteFile(filepath.Join(sbroot, "pre-seal-"+maddr.String()+".key"), []byte(hex.EncodeToString(b)), 0664); err != nil {
			return err
		}
	}

	return nil
}

func createDeals(m *genesis.Miner, k *wallet.Key, maddr address.Address, ssize abi.SectorSize) error {
	for _, sector := range m.Sectors {
		proposal := &market.DealProposal{
			PieceCID:             sector.CommD,
			PieceSize:            abi.PaddedPieceSize(ssize),
			Client:               k.Address,
			Provider:             maddr,
			StartEpoch:           0,
			EndEpoch:             9001,
			StoragePricePerEpoch: big.Zero(),
			ProviderCollateral:   big.Zero(),
			ClientCollateral:     big.Zero(),
		}

		sector.Deal = *proposal
	}

	return nil
}
