package seed

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	logging "github.com/ipfs/go-log/v2"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/minio/blake2b-simd"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-commp-utils/zerocomm"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-storage/storage"

	market2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/market"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper/basicfs"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	"github.com/filecoin-project/lotus/genesis"
)

var log = logging.Logger("preseal")

func PreSeal(maddr address.Address, spt abi.RegisteredSealProof, offset abi.SectorNumber, sectors int, sbroot string, preimage []byte, key *types.KeyInfo, fakeSectors bool) (*genesis.Miner, *types.KeyInfo, error) {
	mid, err := address.IDFromAddress(maddr)
	if err != nil {
		return nil, nil, err
	}

	if err := os.MkdirAll(sbroot, 0775); err != nil { //nolint:gosec
		return nil, nil, err
	}

	next := offset

	sbfs := &basicfs.Provider{
		Root: sbroot,
	}

	sb, err := ffiwrapper.New(sbfs)
	if err != nil {
		return nil, nil, err
	}

	ssize, err := spt.SectorSize()
	if err != nil {
		return nil, nil, err
	}

	var sealedSectors []*genesis.PreSeal
	for i := 0; i < sectors; i++ {
		sid := abi.SectorID{Miner: abi.ActorID(mid), Number: next}
		ref := storage.SectorRef{ID: sid, ProofType: spt}
		next++

		var preseal *genesis.PreSeal
		if !fakeSectors {
			preseal, err = presealSector(sb, sbfs, ref, ssize, preimage)
			if err != nil {
				return nil, nil, err
			}
		} else {
			preseal, err = presealSectorFake(sbfs, ref, ssize)
			if err != nil {
				return nil, nil, err
			}
		}

		sealedSectors = append(sealedSectors, preseal)
	}

	var minerAddr *wallet.Key
	if key != nil {
		minerAddr, err = wallet.NewKey(*key)
		if err != nil {
			return nil, nil, err
		}
	} else {
		minerAddr, err = wallet.GenerateKey(types.KTBLS)
		if err != nil {
			return nil, nil, err
		}
	}

	var pid peer.ID
	{
		log.Warn("PeerID not specified, generating dummy")
		p, _, err := ic.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return nil, nil, err
		}

		pid, err = peer.IDFromPrivateKey(p)
		if err != nil {
			return nil, nil, err
		}
	}

	miner := &genesis.Miner{
		ID:            maddr,
		Owner:         minerAddr.Address,
		Worker:        minerAddr.Address,
		MarketBalance: big.Zero(),
		PowerBalance:  big.Zero(),
		SectorSize:    ssize,
		Sectors:       sealedSectors,
		PeerId:        pid,
	}

	if err := createDeals(miner, minerAddr, maddr, ssize); err != nil {
		return nil, nil, xerrors.Errorf("creating deals: %w", err)
	}

	{
		b, err := json.MarshalIndent(&stores.LocalStorageMeta{
			ID:       stores.ID(uuid.New().String()),
			Weight:   0, // read-only
			CanSeal:  false,
			CanStore: false,
		}, "", "  ")
		if err != nil {
			return nil, nil, xerrors.Errorf("marshaling storage config: %w", err)
		}

		if err := ioutil.WriteFile(filepath.Join(sbroot, "sectorstore.json"), b, 0644); err != nil {
			return nil, nil, xerrors.Errorf("persisting storage metadata (%s): %w", filepath.Join(sbroot, "storage.json"), err)
		}
	}

	return miner, &minerAddr.KeyInfo, nil
}

func presealSector(sb *ffiwrapper.Sealer, sbfs *basicfs.Provider, sid storage.SectorRef, ssize abi.SectorSize, preimage []byte) (*genesis.PreSeal, error) {
	pi, err := sb.AddPiece(context.TODO(), sid, nil, abi.PaddedPieceSize(ssize).Unpadded(), rand.Reader)
	if err != nil {
		return nil, err
	}

	trand := blake2b.Sum256(preimage)
	ticket := abi.SealRandomness(trand[:])

	fmt.Printf("sector-id: %d, piece info: %v\n", sid, pi)

	in2, err := sb.SealPreCommit1(context.TODO(), sid, ticket, []abi.PieceInfo{pi})
	if err != nil {
		return nil, xerrors.Errorf("commit: %w", err)
	}

	cids, err := sb.SealPreCommit2(context.TODO(), sid, in2)
	if err != nil {
		return nil, xerrors.Errorf("commit: %w", err)
	}

	if err := sb.FinalizeSector(context.TODO(), sid, nil); err != nil {
		return nil, xerrors.Errorf("trim cache: %w", err)
	}

	if err := cleanupUnsealed(sbfs, sid); err != nil {
		return nil, xerrors.Errorf("remove unsealed file: %w", err)
	}

	log.Warn("PreCommitOutput: ", sid, cids.Sealed, cids.Unsealed)

	return &genesis.PreSeal{
		CommR:     cids.Sealed,
		CommD:     cids.Unsealed,
		SectorID:  sid.ID.Number,
		ProofType: sid.ProofType,
	}, nil
}

func presealSectorFake(sbfs *basicfs.Provider, sid storage.SectorRef, ssize abi.SectorSize) (*genesis.PreSeal, error) {
	paths, done, err := sbfs.AcquireSector(context.TODO(), sid, 0, storiface.FTSealed|storiface.FTCache, storiface.PathSealing)
	if err != nil {
		return nil, xerrors.Errorf("acquire unsealed sector: %w", err)
	}
	defer done()

	if err := os.Mkdir(paths.Cache, 0755); err != nil {
		return nil, xerrors.Errorf("mkdir cache: %w", err)
	}

	commr, err := ffi.FauxRep(sid.ProofType, paths.Cache, paths.Sealed)
	if err != nil {
		return nil, xerrors.Errorf("fauxrep: %w", err)
	}

	return &genesis.PreSeal{
		CommR:     commr,
		CommD:     zerocomm.ZeroPieceCommitment(abi.PaddedPieceSize(ssize).Unpadded()),
		SectorID:  sid.ID.Number,
		ProofType: sid.ProofType,
	}, nil
}

func cleanupUnsealed(sbfs *basicfs.Provider, ref storage.SectorRef) error {
	paths, done, err := sbfs.AcquireSector(context.TODO(), ref, storiface.FTUnsealed, storiface.FTNone, storiface.PathSealing)
	if err != nil {
		return err
	}
	defer done()

	return os.Remove(paths.Unsealed)
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
	for i, sector := range m.Sectors {
		proposal := &market2.DealProposal{
			PieceCID:             sector.CommD,
			PieceSize:            abi.PaddedPieceSize(ssize),
			Client:               k.Address,
			Provider:             maddr,
			Label:                fmt.Sprintf("%d", i),
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
