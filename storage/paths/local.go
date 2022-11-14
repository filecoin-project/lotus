package paths

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"math/bits"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/lib/result"
	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type LocalStorage interface {
	GetStorage() (storiface.StorageConfig, error)
	SetStorage(func(*storiface.StorageConfig)) error

	Stat(path string) (fsutil.FsStat, error)

	// returns real disk usage for a file/directory
	// os.ErrNotExit when file doesn't exist
	DiskUsage(path string) (int64, error)
}

const MetaFile = "sectorstore.json"

type Local struct {
	localStorage LocalStorage
	index        SectorIndex
	urls         []string

	paths map[storiface.ID]*path

	localLk sync.RWMutex
}

type path struct {
	local      string // absolute local path
	maxStorage uint64

	reserved     int64
	reservations map[abi.SectorID]storiface.SectorFileType
}

func (p *path) stat(ls LocalStorage) (fsutil.FsStat, error) {
	start := time.Now()

	stat, err := ls.Stat(p.local)
	if err != nil {
		return fsutil.FsStat{}, xerrors.Errorf("stat %s: %w", p.local, err)
	}

	stat.Reserved = p.reserved

	for id, ft := range p.reservations {
		for _, fileType := range storiface.PathTypes {
			if fileType&ft == 0 {
				continue
			}

			sp := p.sectorPath(id, fileType)

			used, err := ls.DiskUsage(sp)
			if err == os.ErrNotExist {
				p, ferr := tempFetchDest(sp, false)
				if ferr != nil {
					return fsutil.FsStat{}, ferr
				}

				used, err = ls.DiskUsage(p)
			}
			if err != nil {
				// we don't care about 'not exist' errors, as storage can be
				// reserved before any files are written, so this error is not
				// unexpected
				if !os.IsNotExist(err) {
					log.Warnf("getting disk usage of '%s': %+v", p.sectorPath(id, fileType), err)
				}
				continue
			}

			stat.Reserved -= used
		}
	}

	if stat.Reserved < 0 {
		log.Warnf("negative reserved storage: p.reserved=%d, reserved: %d", p.reserved, stat.Reserved)
		stat.Reserved = 0
	}

	stat.Available -= stat.Reserved
	if stat.Available < 0 {
		stat.Available = 0
	}

	if p.maxStorage > 0 {
		used, err := ls.DiskUsage(p.local)
		if err != nil {
			return fsutil.FsStat{}, err
		}

		stat.Max = int64(p.maxStorage)
		stat.Used = used

		avail := int64(p.maxStorage) - used
		if uint64(used) > p.maxStorage {
			avail = 0
		}

		if avail < stat.Available {
			stat.Available = avail
		}
	}

	if time.Now().Sub(start) > 5*time.Second {
		log.Warnw("slow storage stat", "took", time.Now().Sub(start), "reservations", len(p.reservations))
	}

	return stat, err
}

func (p *path) sectorPath(sid abi.SectorID, fileType storiface.SectorFileType) string {
	return filepath.Join(p.local, fileType.String(), storiface.SectorName(sid))
}

type URLs []string

func NewLocal(ctx context.Context, ls LocalStorage, index SectorIndex, urls []string) (*Local, error) {
	l := &Local{
		localStorage: newCachedLocalStorage(ls),
		index:        index,
		urls:         urls,

		paths: map[storiface.ID]*path{},
	}
	return l, l.open(ctx)
}

func (st *Local) OpenPath(ctx context.Context, p string) error {
	st.localLk.Lock()
	defer st.localLk.Unlock()

	mb, err := ioutil.ReadFile(filepath.Join(p, MetaFile))
	if err != nil {
		return xerrors.Errorf("reading storage metadata for %s: %w", p, err)
	}

	var meta storiface.LocalStorageMeta
	if err := json.Unmarshal(mb, &meta); err != nil {
		return xerrors.Errorf("unmarshalling storage metadata for %s: %w", p, err)
	}

	if p, exists := st.paths[meta.ID]; exists {
		return xerrors.Errorf("path with ID %s already opened: '%s'", meta.ID, p.local)
	}

	// TODO: Check existing / dedupe

	out := &path{
		local: p,

		maxStorage:   meta.MaxStorage,
		reserved:     0,
		reservations: map[abi.SectorID]storiface.SectorFileType{},
	}

	fst, err := out.stat(st.localStorage)
	if err != nil {
		return err
	}

	err = st.index.StorageAttach(ctx, storiface.StorageInfo{
		ID:         meta.ID,
		URLs:       st.urls,
		Weight:     meta.Weight,
		MaxStorage: meta.MaxStorage,
		CanSeal:    meta.CanSeal,
		CanStore:   meta.CanStore,
		Groups:     meta.Groups,
		AllowTo:    meta.AllowTo,
		AllowTypes: meta.AllowTypes,
		DenyTypes:  meta.DenyTypes,
	}, fst)
	if err != nil {
		return xerrors.Errorf("declaring storage in index: %w", err)
	}

	if err := st.declareSectors(ctx, p, meta.ID, meta.CanStore, false); err != nil {
		return err
	}

	st.paths[meta.ID] = out

	return nil
}

func (st *Local) ClosePath(ctx context.Context, id storiface.ID) error {
	st.localLk.Lock()
	defer st.localLk.Unlock()

	if _, exists := st.paths[id]; !exists {
		return xerrors.Errorf("path with ID %s isn't opened", id)
	}

	for _, url := range st.urls {
		if err := st.index.StorageDetach(ctx, id, url); err != nil {
			return xerrors.Errorf("dropping path (id='%s' url='%s'): %w", id, url, err)
		}
	}

	delete(st.paths, id)

	return nil
}

func (st *Local) open(ctx context.Context) error {
	cfg, err := st.localStorage.GetStorage()
	if err != nil {
		return xerrors.Errorf("getting local storage config: %w", err)
	}

	for _, path := range cfg.StoragePaths {
		err := st.OpenPath(ctx, path.Path)
		if err != nil {
			return xerrors.Errorf("opening path %s: %w", path.Path, err)
		}
	}

	go st.reportHealth(ctx)

	return nil
}

func (st *Local) Redeclare(ctx context.Context, filterId *storiface.ID, dropMissingDecls bool) error {
	st.localLk.Lock()
	defer st.localLk.Unlock()

	for id, p := range st.paths {
		mb, err := ioutil.ReadFile(filepath.Join(p.local, MetaFile))
		if err != nil {
			return xerrors.Errorf("reading storage metadata for %s: %w", p.local, err)
		}

		var meta storiface.LocalStorageMeta
		if err := json.Unmarshal(mb, &meta); err != nil {
			return xerrors.Errorf("unmarshalling storage metadata for %s: %w", p.local, err)
		}

		fst, err := p.stat(st.localStorage)
		if err != nil {
			return err
		}

		if id != meta.ID {
			log.Errorf("storage path ID changed: %s; %s -> %s", p.local, id, meta.ID)
			continue
		}
		if filterId != nil && *filterId != id {
			continue
		}

		err = st.index.StorageAttach(ctx, storiface.StorageInfo{
			ID:         id,
			URLs:       st.urls,
			Weight:     meta.Weight,
			MaxStorage: meta.MaxStorage,
			CanSeal:    meta.CanSeal,
			CanStore:   meta.CanStore,
			Groups:     meta.Groups,
			AllowTo:    meta.AllowTo,
			AllowTypes: meta.AllowTypes,
			DenyTypes:  meta.DenyTypes,
		}, fst)
		if err != nil {
			return xerrors.Errorf("redeclaring storage in index: %w", err)
		}

		if err := st.declareSectors(ctx, p.local, meta.ID, meta.CanStore, dropMissingDecls); err != nil {
			return xerrors.Errorf("redeclaring sectors: %w", err)
		}
	}

	return nil
}

func (st *Local) declareSectors(ctx context.Context, p string, id storiface.ID, primary, dropMissing bool) error {
	indexed := map[storiface.Decl]struct{}{}
	if dropMissing {
		decls, err := st.index.StorageList(ctx)
		if err != nil {
			return xerrors.Errorf("getting declaration list: %w", err)
		}

		for _, decl := range decls[id] {
			for _, fileType := range decl.SectorFileType.AllSet() {
				indexed[storiface.Decl{
					SectorID:       decl.SectorID,
					SectorFileType: fileType,
				}] = struct{}{}
			}
		}
	}

	for _, t := range storiface.PathTypes {
		ents, err := os.ReadDir(filepath.Join(p, t.String()))
		if err != nil {
			if os.IsNotExist(err) {
				if err := os.MkdirAll(filepath.Join(p, t.String()), 0755); err != nil { // nolint
					return xerrors.Errorf("openPath mkdir '%s': %w", filepath.Join(p, t.String()), err)
				}

				continue
			}
			return xerrors.Errorf("listing %s: %w", filepath.Join(p, t.String()), err)
		}

		for _, ent := range ents {
			if ent.Name() == FetchTempSubdir {
				continue
			}

			sid, err := storiface.ParseSectorID(ent.Name())
			if err != nil {
				return xerrors.Errorf("parse sector id %s: %w", ent.Name(), err)
			}

			delete(indexed, storiface.Decl{
				SectorID:       sid,
				SectorFileType: t,
			})

			if err := st.index.StorageDeclareSector(ctx, id, sid, t, primary); err != nil {
				return xerrors.Errorf("declare sector %d(t:%d) -> %s: %w", sid, t, id, err)
			}
		}
	}

	if len(indexed) > 0 {
		log.Warnw("index contains sectors which are missing in the storage path", "count", len(indexed), "dropMissing", dropMissing)
	}

	if dropMissing {
		for decl := range indexed {
			if err := st.index.StorageDropSector(ctx, id, decl.SectorID, decl.SectorFileType); err != nil {
				return xerrors.Errorf("dropping sector %v from index: %w", decl, err)
			}
		}
	}

	return nil
}

func (st *Local) reportHealth(ctx context.Context) {
	// randomize interval by ~10%
	interval := (HeartbeatInterval*100_000 + time.Duration(rand.Int63n(10_000))) / 100_000

	for {
		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return
		}

		st.reportStorage(ctx)
	}
}

func (st *Local) reportStorage(ctx context.Context) {
	st.localLk.RLock()

	toReport := map[storiface.ID]storiface.HealthReport{}
	for id, p := range st.paths {
		stat, err := p.stat(st.localStorage)
		r := storiface.HealthReport{Stat: stat}
		if err != nil {
			r.Err = err.Error()
		}

		toReport[id] = r
	}

	st.localLk.RUnlock()

	for id, report := range toReport {
		if err := st.index.StorageReportHealth(ctx, id, report); err != nil {
			log.Warnf("error reporting storage health for %s (%+v): %+v", id, report, err)
		}
	}
}

func (st *Local) Reserve(ctx context.Context, sid storiface.SectorRef, ft storiface.SectorFileType, storageIDs storiface.SectorPaths, overheadTab map[storiface.SectorFileType]int) (func(), error) {
	ssize, err := sid.ProofType.SectorSize()
	if err != nil {
		return nil, err
	}

	st.localLk.Lock()

	done := func() {}
	deferredDone := func() { done() }
	defer func() {
		st.localLk.Unlock()
		deferredDone()
	}()

	for _, fileType := range storiface.PathTypes {
		if fileType&ft == 0 {
			continue
		}

		id := storiface.ID(storiface.PathByType(storageIDs, fileType))

		p, ok := st.paths[id]
		if !ok {
			return nil, errPathNotFound
		}

		stat, err := p.stat(st.localStorage)
		if err != nil {
			return nil, xerrors.Errorf("getting local storage stat: %w", err)
		}

		overhead := int64(overheadTab[fileType]) * int64(ssize) / storiface.FSOverheadDen

		if stat.Available < overhead {
			return nil, storiface.Err(storiface.ErrTempAllocateSpace, xerrors.Errorf("can't reserve %d bytes in '%s' (id:%s), only %d available", overhead, p.local, id, stat.Available))
		}

		p.reserved += overhead
		p.reservations[sid.ID] |= fileType

		prevDone := done
		saveFileType := fileType
		done = func() {
			prevDone()

			st.localLk.Lock()
			defer st.localLk.Unlock()

			p.reserved -= overhead
			p.reservations[sid.ID] ^= saveFileType
			if p.reservations[sid.ID] == storiface.FTNone {
				delete(p.reservations, sid.ID)
			}
		}
	}

	deferredDone = func() {}
	return done, nil
}

func (st *Local) AcquireSector(ctx context.Context, sid storiface.SectorRef, existing storiface.SectorFileType, allocate storiface.SectorFileType, pathType storiface.PathType, op storiface.AcquireMode) (storiface.SectorPaths, storiface.SectorPaths, error) {
	if existing|allocate != existing^allocate {
		return storiface.SectorPaths{}, storiface.SectorPaths{}, xerrors.New("can't both find and allocate a sector")
	}

	ssize, err := sid.ProofType.SectorSize()
	if err != nil {
		return storiface.SectorPaths{}, storiface.SectorPaths{}, err
	}

	st.localLk.RLock()
	defer st.localLk.RUnlock()

	var out storiface.SectorPaths
	var storageIDs storiface.SectorPaths

	for _, fileType := range storiface.PathTypes {
		if fileType&existing == 0 {
			continue
		}

		si, err := st.index.StorageFindSector(ctx, sid.ID, fileType, ssize, false)
		if err != nil {
			log.Warnf("finding existing sector %d(t:%d) failed: %+v", sid, fileType, err)
			continue
		}

		for _, info := range si {
			p, ok := st.paths[info.ID]
			if !ok {
				continue
			}

			if p.local == "" { // TODO: can that even be the case?
				continue
			}

			spath := p.sectorPath(sid.ID, fileType)
			storiface.SetPathByType(&out, fileType, spath)
			storiface.SetPathByType(&storageIDs, fileType, string(info.ID))

			existing ^= fileType
			break
		}
	}

	for _, fileType := range storiface.PathTypes {
		if fileType&allocate == 0 {
			continue
		}

		sis, err := st.index.StorageBestAlloc(ctx, fileType, ssize, pathType)
		if err != nil {
			return storiface.SectorPaths{}, storiface.SectorPaths{}, xerrors.Errorf("finding best storage for allocating : %w", err)
		}

		var best string
		var bestID storiface.ID

		for _, si := range sis {
			p, ok := st.paths[si.ID]
			if !ok {
				continue
			}

			if p.local == "" { // TODO: can that even be the case?
				continue
			}

			if (pathType == storiface.PathSealing) && !si.CanSeal {
				continue
			}

			if (pathType == storiface.PathStorage) && !si.CanStore {
				continue
			}

			if !fileType.Allowed(si.AllowTypes, si.DenyTypes) {
				continue
			}

			// TODO: Check free space

			best = p.sectorPath(sid.ID, fileType)
			bestID = si.ID
			break
		}

		if best == "" {
			return storiface.SectorPaths{}, storiface.SectorPaths{}, xerrors.Errorf("couldn't find a suitable path for a sector")
		}

		storiface.SetPathByType(&out, fileType, best)
		storiface.SetPathByType(&storageIDs, fileType, string(bestID))
		allocate ^= fileType
	}

	return out, storageIDs, nil
}

func (st *Local) Local(ctx context.Context) ([]storiface.StoragePath, error) {
	st.localLk.RLock()
	defer st.localLk.RUnlock()

	var out []storiface.StoragePath
	for id, p := range st.paths {
		if p.local == "" {
			continue
		}

		si, err := st.index.StorageInfo(ctx, id)
		if err != nil {
			return nil, xerrors.Errorf("get storage info for %s: %w", id, err)
		}

		out = append(out, storiface.StoragePath{
			ID:        id,
			Weight:    si.Weight,
			LocalPath: p.local,
			CanSeal:   si.CanSeal,
			CanStore:  si.CanStore,
		})
	}

	return out, nil
}

func (st *Local) Remove(ctx context.Context, sid abi.SectorID, typ storiface.SectorFileType, force bool, keepIn []storiface.ID) error {
	if bits.OnesCount(uint(typ)) != 1 {
		return xerrors.New("delete expects one file type")
	}

	si, err := st.index.StorageFindSector(ctx, sid, typ, 0, false)
	if err != nil {
		return xerrors.Errorf("finding existing sector %d(t:%d) failed: %w", sid, typ, err)
	}

	if len(si) == 0 && !force {
		return xerrors.Errorf("can't delete sector %v(%d), not found", sid, typ)
	}

storeLoop:
	for _, info := range si {
		for _, id := range keepIn {
			if id == info.ID {
				continue storeLoop
			}
		}

		if err := st.removeSector(ctx, sid, typ, info.ID); err != nil {
			return err
		}
	}

	return nil
}

func (st *Local) RemoveCopies(ctx context.Context, sid abi.SectorID, typ storiface.SectorFileType) error {
	if bits.OnesCount(uint(typ)) != 1 {
		return xerrors.New("delete expects one file type")
	}

	si, err := st.index.StorageFindSector(ctx, sid, typ, 0, false)
	if err != nil {
		return xerrors.Errorf("finding existing sector %d(t:%d) failed: %w", sid, typ, err)
	}

	var hasPrimary bool
	for _, info := range si {
		if info.Primary {
			hasPrimary = true
			break
		}
	}

	if !hasPrimary {
		log.Warnf("RemoveCopies: no primary copies of sector %v (%s), not removing anything", sid, typ)
		return nil
	}

	for _, info := range si {
		if info.Primary {
			continue
		}

		if err := st.removeSector(ctx, sid, typ, info.ID); err != nil {
			return err
		}
	}

	return nil
}

func (st *Local) removeSector(ctx context.Context, sid abi.SectorID, typ storiface.SectorFileType, storage storiface.ID) error {
	p, ok := st.paths[storage]
	if !ok {
		return nil
	}

	if p.local == "" { // TODO: can that even be the case?
		return nil
	}

	if err := st.index.StorageDropSector(ctx, storage, sid, typ); err != nil {
		return xerrors.Errorf("dropping sector from index: %w", err)
	}

	spath := p.sectorPath(sid, typ)
	log.Infof("remove %s", spath)

	if err := os.RemoveAll(spath); err != nil {
		log.Errorf("removing sector (%v) from %s: %+v", sid, spath, err)
	}

	st.reportStorage(ctx) // report freed space

	return nil
}

func (st *Local) MoveStorage(ctx context.Context, s storiface.SectorRef, types storiface.SectorFileType) error {
	dest, destIds, err := st.AcquireSector(ctx, s, storiface.FTNone, types, storiface.PathStorage, storiface.AcquireMove)
	if err != nil {
		return xerrors.Errorf("acquire dest storage: %w", err)
	}

	src, srcIds, err := st.AcquireSector(ctx, s, types, storiface.FTNone, storiface.PathStorage, storiface.AcquireMove)
	if err != nil {
		return xerrors.Errorf("acquire src storage: %w", err)
	}

	for _, fileType := range storiface.PathTypes {
		if fileType&types == 0 {
			continue
		}

		sst, err := st.index.StorageInfo(ctx, storiface.ID(storiface.PathByType(srcIds, fileType)))
		if err != nil {
			return xerrors.Errorf("failed to get source storage info: %w", err)
		}

		dst, err := st.index.StorageInfo(ctx, storiface.ID(storiface.PathByType(destIds, fileType)))
		if err != nil {
			return xerrors.Errorf("failed to get source storage info: %w", err)
		}

		if sst.ID == dst.ID {
			log.Debugf("not moving %v(%d); src and dest are the same", s, fileType)
			continue
		}

		if sst.CanStore {
			log.Debugf("not moving %v(%d); source supports storage", s, fileType)
			continue
		}

		log.Debugf("moving %v(%d) to storage: %s(se:%t; st:%t) -> %s(se:%t; st:%t)", s, fileType, sst.ID, sst.CanSeal, sst.CanStore, dst.ID, dst.CanSeal, dst.CanStore)

		if err := st.index.StorageDropSector(ctx, storiface.ID(storiface.PathByType(srcIds, fileType)), s.ID, fileType); err != nil {
			return xerrors.Errorf("dropping source sector from index: %w", err)
		}

		if err := move(storiface.PathByType(src, fileType), storiface.PathByType(dest, fileType)); err != nil {
			// TODO: attempt some recovery (check if src is still there, re-declare)
			return xerrors.Errorf("moving sector %v(%d): %w", s, fileType, err)
		}

		if err := st.index.StorageDeclareSector(ctx, storiface.ID(storiface.PathByType(destIds, fileType)), s.ID, fileType, true); err != nil {
			return xerrors.Errorf("declare sector %d(t:%d) -> %s: %w", s, fileType, storiface.ID(storiface.PathByType(destIds, fileType)), err)
		}
	}

	st.reportStorage(ctx) // report space use changes

	return nil
}

var errPathNotFound = xerrors.Errorf("fsstat: path not found")

func (st *Local) FsStat(ctx context.Context, id storiface.ID) (fsutil.FsStat, error) {
	st.localLk.RLock()
	defer st.localLk.RUnlock()

	p, ok := st.paths[id]
	if !ok {
		return fsutil.FsStat{}, errPathNotFound
	}

	return p.stat(st.localStorage)
}

func (st *Local) GenerateSingleVanillaProof(ctx context.Context, minerID abi.ActorID, si storiface.PostSectorChallenge, ppt abi.RegisteredPoStProof) ([]byte, error) {
	sr := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  minerID,
			Number: si.SectorNumber,
		},
		ProofType: si.SealProof,
	}

	var cache, sealed, cacheID, sealedID string

	if si.Update {
		src, si, err := st.AcquireSector(ctx, sr, storiface.FTUpdate|storiface.FTUpdateCache, storiface.FTNone, storiface.PathStorage, storiface.AcquireMove)
		if err != nil {
			return nil, xerrors.Errorf("acquire sector: %w", err)
		}
		cache, sealed = src.UpdateCache, src.Update
		cacheID, sealedID = si.UpdateCache, si.Update
	} else {
		src, si, err := st.AcquireSector(ctx, sr, storiface.FTSealed|storiface.FTCache, storiface.FTNone, storiface.PathStorage, storiface.AcquireMove)
		if err != nil {
			return nil, xerrors.Errorf("acquire sector: %w", err)
		}
		cache, sealed = src.Cache, src.Sealed
		cacheID, sealedID = si.Cache, si.Sealed
	}

	if sealed == "" || cache == "" {
		return nil, errPathNotFound
	}

	psi := ffi.PrivateSectorInfo{
		SectorInfo: proof.SectorInfo{
			SealProof:    si.SealProof,
			SectorNumber: si.SectorNumber,
			SealedCID:    si.SealedCID,
		},
		CacheDirPath:     cache,
		PoStProofType:    ppt,
		SealedSectorPath: sealed,
	}

	start := time.Now()

	resCh := make(chan result.Result[[]byte], 1)
	go func() {
		resCh <- result.Wrap(ffi.GenerateSingleVanillaProof(psi, si.Challenge))
	}()

	select {
	case r := <-resCh:
		return r.Unwrap()
	case <-ctx.Done():
		log.Errorw("failed to generate valilla PoSt proof before context cancellation", "err", ctx.Err(), "duration", time.Now().Sub(start), "cache-id", cacheID, "sealed-id", sealedID, "cache", cache, "sealed", sealed)

		// this will leave the GenerateSingleVanillaProof goroutine hanging, but that's still less bad than failing PoSt
		return nil, xerrors.Errorf("failed to generate vanilla proof before context cancellation: %w", ctx.Err())
	}
}

var _ Store = &Local{}
