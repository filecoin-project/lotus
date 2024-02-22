package lpffi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/filecoin-project/lotus/lib/must"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/KarpelesLab/reflink"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/go-state-types/abi"
	proof2 "github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/provider/lpproof"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/proofpaths"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var log = logging.Logger("lpffi")

/*
type ExternPrecommit2 func(ctx context.Context, sector storiface.SectorRef, cache, sealed string, pc1out storiface.PreCommit1Out) (sealedCID cid.Cid, unsealedCID cid.Cid, err error)

	type ExternalSealer struct {
		PreCommit2 ExternPrecommit2
	}
*/
type SealCalls struct {
	sectors *storageProvider

	/*// externCalls cointain overrides for calling alternative sealing logic
	externCalls ExternalSealer*/
}

func NewSealCalls(st paths.Store, ls *paths.Local, si paths.SectorIndex) *SealCalls {
	return &SealCalls{
		sectors: &storageProvider{
			storage:    st,
			localStore: ls,
			sindex:     si,
		},
	}
}

type storageProvider struct {
	storage    paths.Store
	localStore *paths.Local
	sindex     paths.SectorIndex
}

type ReleaseStorageFunc func() // free storage reservation

type StorageReservation struct {
	Release          ReleaseStorageFunc
	DeclareAllocated func()
	Paths            storiface.SectorPaths
	PathIDs          storiface.SectorPaths
}

type AcquireSettings struct {
	release ReleaseStorageFunc
}

type AcquireOption func(*AcquireSettings)

func WithReservation(release ReleaseStorageFunc) AcquireOption {
	return func(settings *AcquireSettings) {
		settings.release = release
	}
}

func (l *storageProvider) AcquireSector(ctx context.Context, sector storiface.SectorRef, existing storiface.SectorFileType, allocate storiface.SectorFileType, sealing storiface.PathType, opts ...AcquireOption) (storiface.SectorPaths, func(), error) {
	settings := &AcquireSettings{}
	for _, opt := range opts {
		opt(settings)
	}

	paths, storageIDs, err := l.storage.AcquireSector(ctx, sector, existing, allocate, sealing, storiface.AcquireMove)
	if err != nil {
		return storiface.SectorPaths{}, nil, err
	}

	var releaseStorage func() = settings.release

	if releaseStorage == nil {
		releaseStorage, err = l.localStore.Reserve(ctx, sector, allocate, storageIDs, storiface.FSOverheadSeal)
		if err != nil {
			return storiface.SectorPaths{}, nil, xerrors.Errorf("reserving storage space: %w", err)
		}
	}

	log.Debugf("acquired sector %d (e:%d; a:%d): %v", sector, existing, allocate, paths)

	return paths, func() {
		releaseStorage()

		for _, fileType := range storiface.PathTypes {
			if fileType&allocate == 0 {
				continue
			}

			sid := storiface.PathByType(storageIDs, fileType)
			if err := l.sindex.StorageDeclareSector(ctx, storiface.ID(sid), sector.ID, fileType, true); err != nil {
				log.Errorf("declare sector error: %+v", err)
			}
		}
	}, nil
}

func (sb *SealCalls) ReserveSDRStorage(ctx context.Context, sector storiface.SectorRef) (*StorageReservation, error) {
	// storage writelock sector
	lkctx, cancel := context.WithCancel(ctx)

	allocate := storiface.FTCache

	lockAcquireTimuout := time.Second * 10
	lockAcquireTimer := time.NewTimer(lockAcquireTimuout)

	go func() {
		defer cancel()

		select {
		case <-lockAcquireTimer.C:
		case <-ctx.Done():
		}
	}()

	if err := sb.sectors.sindex.StorageLock(lkctx, sector.ID, storiface.FTNone, allocate); err != nil {
		// timer will expire
		return nil, err
	}

	if !lockAcquireTimer.Stop() {
		// timer expired, so lkctx is done, and that means the lock was acquired and dropped..
		return nil, xerrors.Errorf("failed to acquire lock")
	}
	defer func() {
		// make sure we release the sector lock
		lockAcquireTimer.Reset(0)
	}()

	// find anywhere
	//  if found return nil, for now
	s, err := sb.sectors.sindex.StorageFindSector(ctx, sector.ID, allocate, must.One(sector.ProofType.SectorSize()), false)
	if err != nil {
		return nil, err
	}

	lp, err := sb.sectors.localStore.Local(ctx)
	if err != nil {
		return nil, err
	}

	// see if there are any non-local sector files in storage
	for _, info := range s {
		for _, l := range lp {
			if l.ID == info.ID {
				continue
			}

			return nil, xerrors.Errorf("sector cache already exists and isn't in local storage, can't reserve")
		}
	}

	// acquire a path to make a reservation in
	pathsFs, pathIDs, err := sb.sectors.localStore.AcquireSector(ctx, sector, storiface.FTNone, allocate, storiface.PathSealing, storiface.AcquireMove)
	if err != nil {
		return nil, err
	}

	// reserve the space
	release, err := sb.sectors.localStore.Reserve(ctx, sector, allocate, pathIDs, storiface.FSOverheadSeal)
	if err != nil {
		return nil, err
	}

	declareAllocated := func() {
		for _, fileType := range allocate.AllSet() {
			sid := storiface.PathByType(pathIDs, fileType)
			if err := sb.sectors.sindex.StorageDeclareSector(ctx, storiface.ID(sid), sector.ID, fileType, true); err != nil {
				log.Errorf("declare sector error: %+v", err)
			}
		}
	}

	// note: we drop the sector writelock on return; THAT IS INTENTIONAL, this code runs in CanAccept, which doesn't
	// guarantee that the work for this sector will happen on this node; SDR CanAccept just ensures that the node can
	// run the job, harmonytask is what ensures that only one SDR runs at a time
	return &StorageReservation{
		Release:          release,
		DeclareAllocated: declareAllocated,
		Paths:            pathsFs,
		PathIDs:          pathIDs,
	}, nil
}

func (sb *SealCalls) GenerateSDR(ctx context.Context, reservation *StorageReservation, sector storiface.SectorRef, ticket abi.SealRandomness, commKcid cid.Cid) error {
	// prepare SDR params
	commp, err := commcid.CIDToDataCommitmentV1(commKcid)
	if err != nil {
		return xerrors.Errorf("computing commK: %w", err)
	}

	replicaID, err := sector.ProofType.ReplicaId(sector.ID.Miner, sector.ID.Number, ticket, commp)
	if err != nil {
		return xerrors.Errorf("computing replica id: %w", err)
	}

	// acquire sector paths
	var sectorPaths storiface.SectorPaths
	if reservation != nil {
		sectorPaths = reservation.Paths
		// note: GenerateSDR manages the reservation, so we only need to declare at the end
		defer reservation.DeclareAllocated()
	} else {
		var releaseSector func()
		var err error
		sectorPaths, releaseSector, err = sb.sectors.AcquireSector(ctx, sector, storiface.FTNone, storiface.FTCache, storiface.PathSealing)
		if err != nil {
			return xerrors.Errorf("acquiring sector paths: %w", err)
		}
		defer releaseSector()
	}

	// generate new sector key
	err = ffi.GenerateSDR(
		sector.ProofType,
		sectorPaths.Cache,
		replicaID,
	)
	if err != nil {
		return xerrors.Errorf("generating SDR %d (%s): %w", sector.ID.Number, sectorPaths.Unsealed, err)
	}

	return nil
}

func (sb *SealCalls) TreeD(ctx context.Context, sector storiface.SectorRef, size abi.PaddedPieceSize, data io.Reader, unpaddedData bool) (cid.Cid, error) {
	maybeUns := storiface.FTNone
	// todo sectors with data

	paths, releaseSector, err := sb.sectors.AcquireSector(ctx, sector, storiface.FTCache, maybeUns, storiface.PathSealing)
	if err != nil {
		return cid.Undef, xerrors.Errorf("acquiring sector paths: %w", err)
	}
	defer releaseSector()

	return lpproof.BuildTreeD(data, unpaddedData, filepath.Join(paths.Cache, proofpaths.TreeDName), size)
}

func (sb *SealCalls) TreeRC(ctx context.Context, sector storiface.SectorRef, unsealed cid.Cid) (cid.Cid, cid.Cid, error) {
	p1o, err := sb.makePhase1Out(unsealed, sector.ProofType)
	if err != nil {
		return cid.Undef, cid.Undef, xerrors.Errorf("make phase1 output: %w", err)
	}

	paths, releaseSector, err := sb.sectors.AcquireSector(ctx, sector, storiface.FTCache, storiface.FTSealed, storiface.PathSealing)
	if err != nil {
		return cid.Undef, cid.Undef, xerrors.Errorf("acquiring sector paths: %w", err)
	}
	defer releaseSector()

	{
		// create sector-sized file at paths.Sealed; PC2 transforms it into a sealed sector in-place
		ssize, err := sector.ProofType.SectorSize()
		if err != nil {
			return cid.Undef, cid.Undef, xerrors.Errorf("getting sector size: %w", err)
		}

		{
			// copy TreeD prefix to sealed sector, SealPreCommitPhase2 will mutate it in place into the sealed sector

			// first try reflink + truncate, that should be way faster
			err := reflink.Always(filepath.Join(paths.Cache, proofpaths.TreeDName), paths.Sealed)
			if err == nil {
				err = os.Truncate(paths.Sealed, int64(ssize))
				if err != nil {
					return cid.Undef, cid.Undef, xerrors.Errorf("truncating reflinked sealed file: %w", err)
				}
			} else {
				log.Errorw("reflink treed -> sealed failed, falling back to slow copy, use single scratch btrfs or xfs filesystem", "error", err, "sector", sector, "cache", paths.Cache, "sealed", paths.Sealed)

				// fallback to slow copy, copy ssize bytes from treed to sealed
				dst, err := os.OpenFile(paths.Sealed, os.O_WRONLY|os.O_CREATE, 0644)
				if err != nil {
					return cid.Undef, cid.Undef, xerrors.Errorf("opening sealed sector file: %w", err)
				}
				src, err := os.Open(filepath.Join(paths.Cache, proofpaths.TreeDName))
				if err != nil {
					return cid.Undef, cid.Undef, xerrors.Errorf("opening treed sector file: %w", err)
				}

				_, err = io.CopyN(dst, src, int64(ssize))
				derr := dst.Close()
				_ = src.Close()
				if err != nil {
					return cid.Undef, cid.Undef, xerrors.Errorf("copying treed -> sealed: %w", err)
				}
				if derr != nil {
					return cid.Undef, cid.Undef, xerrors.Errorf("closing sealed file: %w", derr)
				}
			}
		}
	}

	return ffi.SealPreCommitPhase2(p1o, paths.Cache, paths.Sealed)
}

func (sb *SealCalls) GenerateSynthPoRep() {
	panic("todo")
}

func (sb *SealCalls) PoRepSnark(ctx context.Context, sn storiface.SectorRef, sealed, unsealed cid.Cid, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness) ([]byte, error) {
	vproof, err := sb.sectors.storage.GeneratePoRepVanillaProof(ctx, sn, sealed, unsealed, ticket, seed)
	if err != nil {
		return nil, xerrors.Errorf("failed to generate vanilla proof: %w", err)
	}

	proof, err := ffi.SealCommitPhase2(vproof, sn.ID.Number, sn.ID.Miner)
	if err != nil {
		return nil, xerrors.Errorf("computing seal proof failed: %w", err)
	}

	ok, err := ffi.VerifySeal(proof2.SealVerifyInfo{
		SealProof:             sn.ProofType,
		SectorID:              sn.ID,
		DealIDs:               nil,
		Randomness:            ticket,
		InteractiveRandomness: seed,
		Proof:                 proof,
		SealedCID:             sealed,
		UnsealedCID:           unsealed,
	})
	if err != nil {
		return nil, xerrors.Errorf("failed to verify proof: %w", err)
	}
	if !ok {
		return nil, xerrors.Errorf("porep failed to validate")
	}

	return proof, nil
}

func (sb *SealCalls) makePhase1Out(unsCid cid.Cid, spt abi.RegisteredSealProof) ([]byte, error) {
	commd, err := commcid.CIDToDataCommitmentV1(unsCid)
	if err != nil {
		return nil, xerrors.Errorf("make uns cid: %w", err)
	}

	type Config struct {
		ID            string `json:"id"`
		Path          string `json:"path"`
		RowsToDiscard int    `json:"rows_to_discard"`
		Size          int    `json:"size"`
	}

	type Labels struct {
		H      *string  `json:"_h"` // proofs want this..
		Labels []Config `json:"labels"`
	}

	var phase1Output struct {
		CommD           [32]byte           `json:"comm_d"`
		Config          Config             `json:"config"` // TreeD
		Labels          map[string]*Labels `json:"labels"`
		RegisteredProof string             `json:"registered_proof"`
	}

	copy(phase1Output.CommD[:], commd)

	phase1Output.Config.ID = "tree-d"
	phase1Output.Config.Path = "/placeholder"
	phase1Output.Labels = map[string]*Labels{}

	switch spt {
	case abi.RegisteredSealProof_StackedDrg2KiBV1_1, abi.RegisteredSealProof_StackedDrg2KiBV1_1_Feat_SyntheticPoRep:
		phase1Output.Config.RowsToDiscard = 0
		phase1Output.Config.Size = 127
		phase1Output.Labels["StackedDrg2KiBV1"] = &Labels{}
		phase1Output.RegisteredProof = "StackedDrg2KiBV1_1"

		for i := 0; i < 2; i++ {
			phase1Output.Labels["StackedDrg2KiBV1"].Labels = append(phase1Output.Labels["StackedDrg2KiBV1"].Labels, Config{
				ID:            fmt.Sprintf("layer-%d", i+1),
				Path:          "/placeholder",
				RowsToDiscard: 0,
				Size:          64,
			})
		}
	case abi.RegisteredSealProof_StackedDrg512MiBV1_1:
		phase1Output.Config.RowsToDiscard = 0
		phase1Output.Config.Size = 33554431
		phase1Output.Labels["StackedDrg512MiBV1"] = &Labels{}
		phase1Output.RegisteredProof = "StackedDrg512MiBV1_1"

		for i := 0; i < 2; i++ {
			phase1Output.Labels["StackedDrg512MiBV1"].Labels = append(phase1Output.Labels["StackedDrg512MiBV1"].Labels, Config{
				ID:            fmt.Sprintf("layer-%d", i+1),
				Path:          "placeholder",
				RowsToDiscard: 0,
				Size:          16777216,
			})
		}

	case abi.RegisteredSealProof_StackedDrg32GiBV1_1:
		phase1Output.Config.RowsToDiscard = 0
		phase1Output.Config.Size = 2147483647
		phase1Output.Labels["StackedDrg32GiBV1"] = &Labels{}
		phase1Output.RegisteredProof = "StackedDrg32GiBV1_1"

		for i := 0; i < 11; i++ {
			phase1Output.Labels["StackedDrg32GiBV1"].Labels = append(phase1Output.Labels["StackedDrg32GiBV1"].Labels, Config{
				ID:            fmt.Sprintf("layer-%d", i+1),
				Path:          "/placeholder",
				RowsToDiscard: 0,
				Size:          1073741824,
			})
		}

	case abi.RegisteredSealProof_StackedDrg64GiBV1_1:
		phase1Output.Config.RowsToDiscard = 0
		phase1Output.Config.Size = 4294967295
		phase1Output.Labels["StackedDrg64GiBV1"] = &Labels{}
		phase1Output.RegisteredProof = "StackedDrg64GiBV1_1"

		for i := 0; i < 11; i++ {
			phase1Output.Labels["StackedDrg64GiBV1"].Labels = append(phase1Output.Labels["StackedDrg64GiBV1"].Labels, Config{
				ID:            fmt.Sprintf("layer-%d", i+1),
				Path:          "/placeholder",
				RowsToDiscard: 0,
				Size:          2147483648,
			})
		}

	default:
		panic("proof type not handled")
	}

	return json.Marshal(phase1Output)
}

func (sb *SealCalls) LocalStorage(ctx context.Context) ([]storiface.StoragePath, error) {
	return sb.sectors.localStore.Local(ctx)
}

func (sb *SealCalls) FinalizeSector(ctx context.Context, sector storiface.SectorRef, keepUnsealed bool) error {
	alloc := storiface.FTNone
	if keepUnsealed {
		// note: In lotus-provider we don't write the unsealed file in any of the previous stages, it's only written here from tree-d
		alloc = storiface.FTUnsealed
	}

	sectorPaths, releaseSector, err := sb.sectors.AcquireSector(ctx, sector, storiface.FTCache, alloc, storiface.PathSealing)
	if err != nil {
		return xerrors.Errorf("acquiring sector paths: %w", err)
	}
	defer releaseSector()

	ssize, err := sector.ProofType.SectorSize()
	if err != nil {
		return xerrors.Errorf("getting sector size: %w", err)
	}

	if keepUnsealed {
		// tree-d contains exactly unsealed data in the prefix, so
		// * we move it to a temp file
		// * we truncate the temp file to the sector size
		// * we move the temp file to the unsealed location

		// move tree-d to temp file
		tempUnsealed := filepath.Join(sectorPaths.Cache, storiface.SectorName(sector.ID))
		if err := os.Rename(filepath.Join(sectorPaths.Cache, proofpaths.TreeDName), tempUnsealed); err != nil {
			return xerrors.Errorf("moving tree-d to temp file: %w", err)
		}

		// truncate sealed file to sector size
		if err := os.Truncate(tempUnsealed, int64(ssize)); err != nil {
			return xerrors.Errorf("truncating unsealed file to sector size: %w", err)
		}

		// move temp file to unsealed location
		if err := paths.Move(tempUnsealed, sectorPaths.Unsealed); err != nil {
			return xerrors.Errorf("move temp unsealed sector to final location (%s -> %s): %w", tempUnsealed, sectorPaths.Unsealed, err)
		}
	}

	if err := ffi.ClearCache(uint64(ssize), sectorPaths.Cache); err != nil {
		return xerrors.Errorf("clearing cache: %w", err)
	}

	return nil
}

func (sb *SealCalls) MoveStorage(ctx context.Context, sector storiface.SectorRef) error {
	// only move the unsealed file if it still exists and needs moving
	moveUnsealed := storiface.FTUnsealed
	{
		found, unsealedPathType, err := sb.sectorStorageType(ctx, sector, storiface.FTUnsealed)
		if err != nil {
			return xerrors.Errorf("checking cache storage type: %w", err)
		}

		if !found || unsealedPathType == storiface.PathStorage {
			moveUnsealed = storiface.FTNone
		}
	}

	toMove := storiface.FTCache | storiface.FTSealed | moveUnsealed

	err := sb.sectors.storage.MoveStorage(ctx, sector, toMove)
	if err != nil {
		return xerrors.Errorf("moving storage: %w", err)
	}

	for _, fileType := range toMove.AllSet() {
		if err := sb.sectors.storage.RemoveCopies(ctx, sector.ID, fileType); err != nil {
			return xerrors.Errorf("rm copies (t:%s, s:%v): %w", fileType, sector, err)
		}
	}

	return nil
}

func (sb *SealCalls) sectorStorageType(ctx context.Context, sector storiface.SectorRef, ft storiface.SectorFileType) (sectorFound bool, ptype storiface.PathType, err error) {
	stores, err := sb.sectors.sindex.StorageFindSector(ctx, sector.ID, ft, 0, false)
	if err != nil {
		return false, "", xerrors.Errorf("finding sector: %w", err)
	}
	if len(stores) == 0 {
		return false, "", nil
	}

	for _, store := range stores {
		if store.CanSeal {
			return true, storiface.PathSealing, nil
		}
	}

	return true, storiface.PathStorage, nil
}
