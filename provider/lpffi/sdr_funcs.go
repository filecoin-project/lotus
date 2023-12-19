package lpffi

import (
	"context"
	ffi "github.com/filecoin-project/filecoin-ffi"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"
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

func (l *storageProvider) AcquireSector(ctx context.Context, sector storiface.SectorRef, existing storiface.SectorFileType, allocate storiface.SectorFileType, sealing storiface.PathType) (storiface.SectorPaths, func(), error) {
	paths, storageIDs, err := l.storage.AcquireSector(ctx, sector, existing, allocate, sealing, storiface.AcquireMove)
	if err != nil {
		return storiface.SectorPaths{}, nil, err
	}

	releaseStorage, err := l.localStore.Reserve(ctx, sector, allocate, storageIDs, storiface.FSOverheadSeal)
	if err != nil {
		return storiface.SectorPaths{}, nil, xerrors.Errorf("reserving storage space: %w", err)
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

func (sb *SealCalls) GenerateSDR(ctx context.Context, sector storiface.SectorRef, ticket abi.SealRandomness, commKcid cid.Cid) error {
	paths, releaseSector, err := sb.sectors.AcquireSector(ctx, sector, storiface.FTCache, storiface.FTNone, storiface.PathSealing)
	if err != nil {
		return xerrors.Errorf("acquiring sector paths: %w", err)
	}
	defer releaseSector()

	// prepare SDR params
	commp, err := commcid.CIDToDataCommitmentV1(commKcid)
	if err != nil {
		return xerrors.Errorf("computing commK: %w", err)
	}

	replicaID, err := sector.ProofType.ReplicaId(sector.ID.Miner, sector.ID.Number, ticket, commp)
	if err != nil {
		return xerrors.Errorf("computing replica id: %w", err)
	}

	// generate new sector key
	err = ffi.GenerateSDR(
		sector.ProofType,
		paths.Cache,
		replicaID,
	)
	if err != nil {
		return xerrors.Errorf("generating SDR %d (%s): %w", sector.ID.Number, paths.Unsealed, err)
	}

	return nil
}
