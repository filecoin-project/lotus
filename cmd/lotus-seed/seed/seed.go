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

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/crypto"
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
		Miner:           maddr,
		SealProofType:   spt,
		PoStProofType:   ppt,
		FallbackLastNum: offset,
		Paths:           sectorbuilder.SimplePath(sbroot),
		WorkerThreads:   2,
	}

	if err := os.MkdirAll(sbroot, 0775); err != nil {
		return nil, nil, err
	}

	mds, err := badger.NewDatastore(filepath.Join(sbroot, "badger"), nil)
	if err != nil {
		return nil, nil, err
	}

	sb, err := sectorbuilder.New(cfg, namespace.Wrap(mds, datastore.NewKey("/sectorbuilder")))
	if err != nil {
		return nil, nil, err
	}

	ssize, err := pt.SectorSize()
	if err != nil {
		return nil, nil, err
	}

	var sealedSectors []*genesis.PreSeal
	for i := 0; i < sectors; i++ {
		sid, err := sb.AcquireSectorNumber()
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

		scid, ucid, err := sb.SealPreCommit(context.TODO(), sid, ticket, []abi.PieceInfo{pi})
		if err != nil {
			return nil, nil, xerrors.Errorf("commit: %w", err)
		}

		if err := sb.TrimCache(context.TODO(), sid); err != nil {
			return nil, nil, xerrors.Errorf("trim cache: %w", err)
		}

		log.Warn("PreCommitOutput: ", sid, scid, ucid)
		sealedSectors = append(sealedSectors, &genesis.PreSeal{
			CommR:    scid,
			CommD:    ucid,
			SectorID: sid,
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

	if err := mds.Close(); err != nil {
		return nil, nil, xerrors.Errorf("closing datastore: %w", err)
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
