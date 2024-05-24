package api

import (
	"context"
	"net/http"
	"net/url"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	lpiece "github.com/filecoin-project/lotus/storage/pipeline/piece"
	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type Curio interface {
	Version(context.Context) (Version, error) //perm:admin

	AllocatePieceToSector(ctx context.Context, maddr address.Address, piece lpiece.PieceDealInfo, rawSize int64, source url.URL, header http.Header) (SectorOffset, error) //perm:write

	StorageInit(ctx context.Context, path string, opts storiface.LocalStorageMeta) error                                                                                   //perm:admin
	StorageAddLocal(ctx context.Context, path string) error                                                                                                                //perm:admin
	StorageDetachLocal(ctx context.Context, path string) error                                                                                                             //perm:admin
	StorageList(ctx context.Context) (map[storiface.ID][]storiface.Decl, error)                                                                                            //perm:admin
	StorageLocal(ctx context.Context) (map[storiface.ID]string, error)                                                                                                     //perm:admin
	StorageStat(ctx context.Context, id storiface.ID) (fsutil.FsStat, error)                                                                                               //perm:admin
	StorageInfo(context.Context, storiface.ID) (storiface.StorageInfo, error)                                                                                              //perm:admin
	StorageFindSector(ctx context.Context, sector abi.SectorID, ft storiface.SectorFileType, ssize abi.SectorSize, allowFetch bool) ([]storiface.SectorStorageInfo, error) //perm:admin

	LogList(ctx context.Context) ([]string, error)                  //perm:read
	LogSetLevel(ctx context.Context, subsystem, level string) error //perm:admin

	// Trigger shutdown
	Shutdown(context.Context) error //perm:admin
}
