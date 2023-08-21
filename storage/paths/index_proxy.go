package paths

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/journal/alerting"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type IndexProxy struct {
	memIndex            *Index
	dbIndex             *DBIndex
	enableSectorIndexDB bool
}

func NewIndexProxyHelper(enableSectorIndexDB bool) func(al *alerting.Alerting, db *harmonydb.DB) *IndexProxy {
	return func(al *alerting.Alerting, db *harmonydb.DB) *IndexProxy {
		return NewIndexProxy(al, db, enableSectorIndexDB)
	}
}

func NewIndexProxy(al *alerting.Alerting, db *harmonydb.DB, enableSectorIndexDB bool) *IndexProxy {
	return &IndexProxy{
		memIndex:            NewIndex(al),
		dbIndex:             NewDBIndex(al, db),
		enableSectorIndexDB: enableSectorIndexDB,
	}
}

func (ip *IndexProxy) StorageAttach(ctx context.Context, info storiface.StorageInfo, stat fsutil.FsStat) error {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageAttach(ctx, info, stat)
	}
	return ip.memIndex.StorageAttach(ctx, info, stat)
}

func (ip *IndexProxy) StorageDetach(ctx context.Context, id storiface.ID, url string) error {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageDetach(ctx, id, url)
	}
	return ip.memIndex.StorageDetach(ctx, id, url)
}

func (ip *IndexProxy) StorageInfo(ctx context.Context, id storiface.ID) (storiface.StorageInfo, error) {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageInfo(ctx, id)
	}
	return ip.memIndex.StorageInfo(ctx, id)
}

func (ip *IndexProxy) StorageReportHealth(ctx context.Context, id storiface.ID, report storiface.HealthReport) error {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageReportHealth(ctx, id, report)
	}
	return ip.memIndex.StorageReportHealth(ctx, id, report)
}

func (ip *IndexProxy) StorageDeclareSector(ctx context.Context, storageID storiface.ID, s abi.SectorID, ft storiface.SectorFileType, primary bool) error {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageDeclareSector(ctx, storageID, s, ft, primary)
	}
	return ip.memIndex.StorageDeclareSector(ctx, storageID, s, ft, primary)
}

func (ip *IndexProxy) StorageDropSector(ctx context.Context, storageID storiface.ID, s abi.SectorID, ft storiface.SectorFileType) error {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageDropSector(ctx, storageID, s, ft)
	}
	return ip.memIndex.StorageDropSector(ctx, storageID, s, ft)
}

func (ip *IndexProxy) StorageFindSector(ctx context.Context, sector abi.SectorID, ft storiface.SectorFileType, ssize abi.SectorSize, allowFetch bool) ([]storiface.SectorStorageInfo, error) {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageFindSector(ctx, sector, ft, ssize, allowFetch)
	}
	return ip.memIndex.StorageFindSector(ctx, sector, ft, ssize, allowFetch)
}

func (ip *IndexProxy) StorageBestAlloc(ctx context.Context, allocate storiface.SectorFileType, ssize abi.SectorSize, pathType storiface.PathType) ([]storiface.StorageInfo, error) {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageBestAlloc(ctx, allocate, ssize, pathType)
	}
	return ip.memIndex.StorageBestAlloc(ctx, allocate, ssize, pathType)
}

func (ip *IndexProxy) StorageLock(ctx context.Context, sector abi.SectorID, read storiface.SectorFileType, write storiface.SectorFileType) error {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageLock(ctx, sector, read, write)
	}
	return ip.memIndex.StorageLock(ctx, sector, read, write)
}

func (ip *IndexProxy) StorageTryLock(ctx context.Context, sector abi.SectorID, read storiface.SectorFileType, write storiface.SectorFileType) (bool, error) {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageTryLock(ctx, sector, read, write)
	}
	return ip.memIndex.StorageTryLock(ctx, sector, read, write)
}

func (ip *IndexProxy) StorageGetLocks(ctx context.Context) (storiface.SectorLocks, error) {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageGetLocks(ctx)
	}
	return ip.memIndex.StorageGetLocks(ctx)
}

func (ip *IndexProxy) StorageList(ctx context.Context) (map[storiface.ID][]storiface.Decl, error) {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageList(ctx)
	}
	return ip.memIndex.StorageList(ctx)
}

var _ SectorIndex = &IndexProxy{}
