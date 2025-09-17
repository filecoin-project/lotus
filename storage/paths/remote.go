package paths

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/bits"
	"net/http"
	"net/url"
	"os"
	gopath "path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/partialfile"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var FetchTempSubdir = "fetching"

var CopyBuf = 1 << 20

// LocalReaderTimeout is the timeout for keeping local reader files open without
// any read activity.
var LocalReaderTimeout = 5 * time.Second

type Remote struct {
	local Store
	index SectorIndex
	auth  http.Header

	limit chan struct{}

	fetchLk  sync.Mutex
	fetching map[abi.SectorID]chan struct{}

	pfHandler PartialFileHandler
}

func (r *Remote) RemoveCopies(ctx context.Context, s abi.SectorID, typ storiface.SectorFileType) error {
	if bits.OnesCount(uint(typ)) != 1 {
		return xerrors.New("RemoveCopies expects one file type")
	}

	if err := r.local.RemoveCopies(ctx, s, typ); err != nil {
		return xerrors.Errorf("removing local copies: %w", err)
	}

	si, err := r.index.StorageFindSector(ctx, s, typ, 0, false)
	if err != nil {
		return xerrors.Errorf("finding existing sector %d(t:%d) failed: %w", s, typ, err)
	}

	var hasPrimary bool
	var keep []storiface.ID
	for _, info := range si {
		if info.Primary {
			hasPrimary = true
			keep = append(keep, info.ID)
			break
		}
	}

	if !hasPrimary {
		log.Warnf("remote RemoveCopies: no primary copies of sector %v (%s), not removing anything", s, typ)
		return nil
	}

	return r.Remove(ctx, s, typ, true, keep)
}

func NewRemote(local Store, index SectorIndex, auth http.Header, fetchLimit int, pfHandler PartialFileHandler) *Remote {
	return &Remote{
		local: local,
		index: index,
		auth:  auth,

		limit: make(chan struct{}, fetchLimit),

		fetching:  map[abi.SectorID]chan struct{}{},
		pfHandler: pfHandler,
	}
}

func (r *Remote) AcquireSector(ctx context.Context, s storiface.SectorRef, existing storiface.SectorFileType, allocate storiface.SectorFileType, pathType storiface.PathType, op storiface.AcquireMode, opts ...storiface.AcquireOption) (storiface.SectorPaths, storiface.SectorPaths, error) {
	if existing|allocate != existing^allocate {
		return storiface.SectorPaths{}, storiface.SectorPaths{}, xerrors.New("can't both find and allocate a sector")
	}

	settings := storiface.AcquireSettings{
		// Into will tell us which paths things should be fetched into or allocated in.
		Into: nil,
	}
	for _, o := range opts {
		o(&settings)
	}

	if settings.Into != nil {
		if !allocate.IsNone() {
			return storiface.SectorPaths{}, storiface.SectorPaths{}, xerrors.New("cannot specify Into with allocate")
		}
		if !settings.Into.HasAllSet(existing) {
			return storiface.SectorPaths{}, storiface.SectorPaths{}, xerrors.New("Into has to have all existing paths")
		}
	}

	// First make sure that no other goroutines are trying to fetch this sector;
	// wait if there are any.
	for {
		r.fetchLk.Lock()

		c, locked := r.fetching[s.ID]
		if !locked {
			r.fetching[s.ID] = make(chan struct{})
			r.fetchLk.Unlock()
			break
		}

		r.fetchLk.Unlock()

		select {
		case <-c:
			continue
		case <-ctx.Done():
			return storiface.SectorPaths{}, storiface.SectorPaths{}, ctx.Err()
		}
	}

	defer func() {
		r.fetchLk.Lock()
		close(r.fetching[s.ID])
		delete(r.fetching, s.ID)
		r.fetchLk.Unlock()
	}()

	// Try to get the sector from local storage
	paths, stores, err := r.local.AcquireSector(ctx, s, existing, allocate, pathType, op)
	if err != nil {
		return storiface.SectorPaths{}, storiface.SectorPaths{}, xerrors.Errorf("local acquire error: %w", err)
	}

	var toFetch storiface.SectorFileType
	for _, fileType := range existing.AllSet() {
		if storiface.PathByType(paths, fileType) == "" {
			toFetch |= fileType
		}
	}

	// get a list of paths to fetch data into. Note: file type filters will apply inside this call.
	var fetchPaths, fetchIDs storiface.SectorPaths

	if settings.Into == nil {
		// fetching without existing reservation, so allocate paths and create a reservation
		fetchPaths, fetchIDs, err = r.local.AcquireSector(ctx, s, storiface.FTNone, toFetch, pathType, op)
		if err != nil {
			return storiface.SectorPaths{}, storiface.SectorPaths{}, xerrors.Errorf("allocate local sector for fetching: %w", err)
		}

		log.Debugw("Fetching sector data without existing reservation", "sector", s, "toFetch", toFetch, "fetchPaths", fetchPaths, "fetchIDs", fetchIDs)

		overheadTable := storiface.FSOverheadSeal
		if pathType == storiface.PathStorage {
			overheadTable = storiface.FsOverheadFinalized
		}

		// If any path types weren't found in local storage, try fetching them

		// First reserve storage
		releaseStorage, err := r.local.Reserve(ctx, s, toFetch, fetchIDs, overheadTable, MinFreeStoragePercentage)
		if err != nil {
			return storiface.SectorPaths{}, storiface.SectorPaths{}, xerrors.Errorf("reserving storage space: %w", err)
		}
		defer releaseStorage()
	} else {
		fetchPaths = settings.Into.Paths
		fetchIDs = settings.Into.IDs

		log.Debugw("Fetching sector data with existing reservation", "sector", s, "toFetch", toFetch, "fetchPaths", fetchPaths, "fetchIDs", fetchIDs)
	}

	for _, fileType := range toFetch.AllSet() {
		dest := storiface.PathByType(fetchPaths, fileType)
		storageID := storiface.PathByType(fetchIDs, fileType)

		url, err := r.acquireFromRemote(ctx, s.ID, fileType, dest)
		if err != nil {
			return storiface.SectorPaths{}, storiface.SectorPaths{}, err
		}

		storiface.SetPathByType(&paths, fileType, dest)
		storiface.SetPathByType(&stores, fileType, storageID)

		if err := r.index.StorageDeclareSector(ctx, storiface.ID(storageID), s.ID, fileType, op == storiface.AcquireMove); err != nil {
			log.Warnf("declaring sector %v in %s failed: %+v", s, storageID, err)
			continue
		}

		if op == storiface.AcquireMove {
			id := storiface.ID(storageID)
			if err := r.deleteFromRemote(ctx, url, []storiface.ID{id}); err != nil {
				log.Warnf("deleting sector %v from %s (delete %s): %+v", s, storageID, url, err)
			}
		}
	}

	return paths, stores, nil
}

func tempFetchDest(spath string, create bool) (string, error) {
	st, b := filepath.Split(spath)
	tempdir := filepath.Join(st, FetchTempSubdir)
	if create {
		if err := os.MkdirAll(tempdir, 0755); err != nil { // nolint
			return "", xerrors.Errorf("creating temp fetch dir: %w", err)
		}
	}

	return filepath.Join(tempdir, b), nil
}

func (r *Remote) acquireFromRemote(ctx context.Context, s abi.SectorID, fileType storiface.SectorFileType, dest string) (string, error) {
	si, err := r.index.StorageFindSector(ctx, s, fileType, 0, false)
	if err != nil {
		return "", err
	}

	if len(si) == 0 {
		return "", xerrors.Errorf("failed to acquire sector %v from remote(%d): %w", s, fileType, storiface.ErrSectorNotFound)
	}

	sort.Slice(si, func(i, j int) bool {
		return si[i].Weight < si[j].Weight
	})

	var merr error
	for _, info := range si {
		// TODO: see what we have local, prefer that

		for _, url := range info.URLs {
			tempDest, err := tempFetchDest(dest, true)
			if err != nil {
				return "", err
			}

			if err := os.RemoveAll(dest); err != nil {
				return "", xerrors.Errorf("removing dest: %w", err)
			}

			err = r.fetchThrottled(ctx, url, tempDest)
			if err != nil {
				merr = multierror.Append(merr, xerrors.Errorf("fetch error %s (storage %s) -> %s: %w", url, info.ID, tempDest, err))
				// fetching failed, remove temp file
				if rerr := os.RemoveAll(tempDest); rerr != nil {
					merr = multierror.Append(merr, xerrors.Errorf("removing temp dest (post-err cleanup): %w", rerr))
				}
				continue
			}

			if err := Move(tempDest, dest); err != nil {
				return "", xerrors.Errorf("fetch move error (storage %s) %s -> %s: %w", info.ID, tempDest, dest, err)
			}

			if merr != nil {
				log.Warnw("acquireFromRemote encountered errors when fetching sector from remote", "errors", merr)
			}
			return url, nil
		}
	}

	return "", xerrors.Errorf("failed to acquire sector %v from remote (tried %v): %w", s, si, merr)
}

func (r *Remote) fetchThrottled(ctx context.Context, url, outname string) (rerr error) {
	if len(r.limit) >= cap(r.limit) {
		log.Infof("Throttling fetch, %d already running", len(r.limit))
	}

	// TODO: Smarter throttling
	//  * Priority (just going sequentially is still pretty good)
	//  * Per interface
	//  * Aware of remote load
	select {
	case r.limit <- struct{}{}:
		defer func() { <-r.limit }()
	case <-ctx.Done():
		return xerrors.Errorf("context error while waiting for fetch limiter: %w", ctx.Err())
	}

	return fetch(ctx, url, outname, r.auth)
}

func (r *Remote) checkAllocated(ctx context.Context, url string, spt abi.RegisteredSealProof, offset, size abi.PaddedPieceSize) (bool, error) {
	url = fmt.Sprintf("%s/%d/allocated/%d/%d", url, spt, offset.Unpadded(), size.Unpadded())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, xerrors.Errorf("request: %w", err)
	}
	req.Header = r.auth.Clone()
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, xerrors.Errorf("do request: %w", err)
	}
	defer resp.Body.Close() // nolint

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusRequestedRangeNotSatisfiable:
		return false, nil
	default:
		return false, xerrors.Errorf("unexpected http response: %d", resp.StatusCode)
	}
}

func (r *Remote) MoveStorage(ctx context.Context, s storiface.SectorRef, types storiface.SectorFileType, opts ...storiface.AcquireOption) error {
	// Make sure we have the data local
	_, _, err := r.AcquireSector(ctx, s, types, storiface.FTNone, storiface.PathStorage, storiface.AcquireMove, opts...)
	if err != nil {
		return xerrors.Errorf("acquire src storage (remote): %w", err)
	}

	return r.local.MoveStorage(ctx, s, types, opts...)
}

func (r *Remote) Remove(ctx context.Context, sid abi.SectorID, typ storiface.SectorFileType, force bool, keepIn []storiface.ID) error {
	if bits.OnesCount(uint(typ)) != 1 {
		return xerrors.New("delete expects one file type")
	}

	if err := r.local.Remove(ctx, sid, typ, force, keepIn); err != nil {
		return xerrors.Errorf("remove from local: %w", err)
	}

	si, err := r.index.StorageFindSector(ctx, sid, typ, 0, false)
	if err != nil {
		return xerrors.Errorf("finding existing sector %d(t:%d) failed: %w", sid, typ, err)
	}

storeLoop:
	for _, info := range si {
		for _, id := range keepIn {
			if id == info.ID {
				continue storeLoop
			}
		}
		for _, url := range info.URLs {
			if err := r.deleteFromRemote(ctx, url, keepIn); err != nil {
				log.Warnf("remove %s: %+v", url, err)
				continue
			}
			break
		}
	}

	return nil
}

func (r *Remote) deleteFromRemote(ctx context.Context, url string, keepIn storiface.IDList) error {
	if keepIn != nil {
		url = url + "?keep=" + keepIn.String()
	}

	log.Infof("Delete %s", url)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return xerrors.Errorf("request: %w", err)
	}
	req.Header = r.auth
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return xerrors.Errorf("do request: %w", err)
	}
	defer resp.Body.Close() // nolint

	if resp.StatusCode != 200 {
		return xerrors.Errorf("non-200 code: %d", resp.StatusCode)
	}

	return nil
}

func (r *Remote) FsStat(ctx context.Context, id storiface.ID) (fsutil.FsStat, error) {
	st, err := r.local.FsStat(ctx, id)
	switch err {
	case nil:
		return st, nil
	case errPathNotFound:
		break
	default:
		return fsutil.FsStat{}, xerrors.Errorf("local stat: %w", err)
	}

	si, err := r.index.StorageInfo(ctx, id)
	if err != nil {
		return fsutil.FsStat{}, xerrors.Errorf("getting remote storage info: %w", err)
	}

	if len(si.URLs) == 0 {
		return fsutil.FsStat{}, xerrors.Errorf("no known URLs for remote storage %s", id)
	}

	for _, urlStr := range si.URLs {
		out, err := r.StatUrl(ctx, urlStr, id)
		if err != nil {
			log.Warnw("stat url failed", "url", urlStr, "error", err)
			continue
		}

		return out, nil
	}

	return fsutil.FsStat{}, xerrors.Errorf("all endpoints failed for remote storage %s", id)
}

func (r *Remote) StatUrl(ctx context.Context, urlStr string, id storiface.ID) (fsutil.FsStat, error) {
	rl, err := url.Parse(urlStr)
	if err != nil {
		return fsutil.FsStat{}, xerrors.Errorf("parsing URL: %w", err)
	}

	rl.Path = gopath.Join(rl.Path, "stat", string(id))

	req, err := http.NewRequest("GET", rl.String(), nil)
	if err != nil {
		return fsutil.FsStat{}, xerrors.Errorf("creating request failed: %w", err)
	}
	req.Header = r.auth
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fsutil.FsStat{}, xerrors.Errorf("do request: %w", err)
	}

	if resp.StatusCode == 200 {
		var out fsutil.FsStat
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			_ = resp.Body.Close()
			return fsutil.FsStat{}, xerrors.Errorf("decoding response failed: %w", err)
		}
		_ = resp.Body.Close()
		return out, nil // Successfully decoded, return the result
	}

	// non-200 status code
	b, _ := io.ReadAll(resp.Body) // Best-effort read the body for logging
	_ = resp.Body.Close()

	return fsutil.FsStat{}, xerrors.Errorf("endpoint failed %s: %d %s", rl.String(), resp.StatusCode, string(b))
}

func (r *Remote) readRemote(ctx context.Context, url string, offset, size abi.PaddedPieceSize) (io.ReadCloser, error) {
	if len(r.limit) >= cap(r.limit) {
		log.Infof("Throttling remote read, %d already running", len(r.limit))
	}

	// TODO: Smarter throttling
	//  * Priority (just going sequentially is still pretty good)
	//  * Per interface
	//  * Aware of remote load
	select {
	case r.limit <- struct{}{}:
		defer func() { <-r.limit }()
	case <-ctx.Done():
		return nil, xerrors.Errorf("context error while waiting for fetch limiter: %w", ctx.Err())
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, xerrors.Errorf("request: %w", err)
	}

	if r.auth != nil {
		req.Header = r.auth.Clone()
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+size-1))
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, xerrors.Errorf("do request: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close() // nolint
		return nil, xerrors.Errorf("non-200 code: %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// CheckIsUnsealed checks if we have an unsealed piece at the given offset in an already unsealed sector file for the given piece
// either locally or on any of the workers.
// Returns true if we have the unsealed piece, false otherwise.
func (r *Remote) CheckIsUnsealed(ctx context.Context, s storiface.SectorRef, offset, size abi.PaddedPieceSize) (bool, error) {
	ft := storiface.FTUnsealed

	paths, _, err := r.local.AcquireSector(ctx, s, ft, storiface.FTNone, storiface.PathStorage, storiface.AcquireMove)
	if err != nil {
		return false, xerrors.Errorf("acquire local: %w", err)
	}

	path := storiface.PathByType(paths, ft)
	if path != "" {
		// if we have the unsealed file locally, check if it has the unsealed piece.
		log.Infof("Read local %s (+%d,%d)", path, offset, size)
		ssize, err := s.ProofType.SectorSize()
		if err != nil {
			return false, err
		}

		// open the unsealed sector file for the given sector size located at the given path.
		pf, err := r.pfHandler.OpenPartialFile(abi.PaddedPieceSize(ssize), path)
		if err != nil {
			return false, xerrors.Errorf("opening partial file: %w", err)
		}
		log.Debugf("local partial file opened %s (+%d,%d)", path, offset, size)

		// even though we have an unsealed file for the given sector, we still need to determine if we have the unsealed piece
		// in the unsealed sector file. That is what `HasAllocated` checks for.
		has, err := r.pfHandler.HasAllocated(pf, storiface.UnpaddedByteIndex(offset.Unpadded()), size.Unpadded())
		if err != nil {
			return false, xerrors.Errorf("has allocated: %w", err)
		}

		// close the local unsealed file.
		if err := r.pfHandler.Close(pf); err != nil {
			return false, xerrors.Errorf("failed to close partial file: %s", err)
		}
		log.Debugf("checked if local partial file has the piece %s (+%d,%d), returning answer=%t", path, offset, size, has)

		// Sector files can technically not have a piece unsealed locally, but have it unsealed in remote storage, so we probably
		// want to return only if has is true
		if has {
			return has, nil
		}
	}

	// --- We don't have the unsealed piece in an unsealed sector file locally
	// Check if we have it in a remote cluster.

	si, err := r.index.StorageFindSector(ctx, s.ID, ft, 0, false)
	if err != nil {
		return false, xerrors.Errorf("StorageFindSector: %s", err)
	}

	if len(si) == 0 {
		return false, nil
	}

	sort.Slice(si, func(i, j int) bool {
		return si[i].Weight < si[j].Weight
	})

	for _, info := range si {
		for _, url := range info.URLs {
			ok, err := r.checkAllocated(ctx, url, s.ProofType, offset, size)
			if err != nil {
				log.Warnw("check if remote has piece", "url", url, "error", err)
				continue
			}
			if !ok {
				continue
			}

			return true, nil
		}
	}

	return false, nil
}

// Reader returns a reader for an unsealed piece at the given offset in the given sector.
// If the Miner has the unsealed piece locally, it will return a reader that reads from the local copy.
// If the Miner does NOT have the unsealed piece locally, it will query all workers that have the unsealed sector file
// to know if they have the unsealed piece and will then read the unsealed piece data from a worker that has it.
//
// Returns a nil reader if :
// 1. no worker(local worker included) has an unsealed file for the given sector OR
// 2. no worker(local worker included) has the unsealed piece in their unsealed sector file.
// Will return a nil reader and a nil error in such a case.
func (r *Remote) Reader(ctx context.Context, s storiface.SectorRef, offset, size abi.PaddedPieceSize) (func(startOffsetAligned, endOffsetAligned storiface.PaddedByteIndex) (io.ReadCloser, error), error) {
	ft := storiface.FTUnsealed

	// check if we have the unsealed sector file locally
	paths, _, err := r.local.AcquireSector(ctx, s, ft, storiface.FTNone, storiface.PathStorage, storiface.AcquireMove)
	if err != nil {
		return nil, xerrors.Errorf("acquire local: %w", err)
	}

	path := storiface.PathByType(paths, ft)

	if path != "" {
		// if we have the unsealed file locally, return a reader that can be used to read the contents of the
		// unsealed piece.
		log.Debugf("Check local %s (+%d,%d)", path, offset, size)
		ssize, err := s.ProofType.SectorSize()
		if err != nil {
			return nil, err
		}
		log.Debugf("fetched sector size %s (+%d,%d)", path, offset, size)

		// open the unsealed sector file for the given sector size located at the given path.
		pf, err := r.pfHandler.OpenPartialFile(abi.PaddedPieceSize(ssize), path)
		if err != nil {
			return nil, xerrors.Errorf("opening partial file: %w", err)
		}
		log.Debugf("local partial file opened %s (+%d,%d)", path, offset, size)

		// even though we have an unsealed file for the given sector, we still need to determine if we have the unsealed piece
		// in the unsealed sector file. That is what `HasAllocated` checks for.
		has, err := r.pfHandler.HasAllocated(pf, storiface.UnpaddedByteIndex(offset.Unpadded()), size.Unpadded())
		if err != nil {
			return nil, xerrors.Errorf("has allocated: %w", err)
		}
		log.Debugf("check if partial file is allocated %s (+%d,%d)", path, offset, size)

		if has {
			log.Infof("returning piece reader for local unsealed piece sector=%+v, (offset=%d, size=%d)", s.ID, offset, size)

			// refs keep track of the currently opened pf
			// if they drop to 0 for longer than LocalReaderTimeout, pf will be closed
			var refsLk sync.Mutex
			refs := 0

			cleanupIdle := func() {
				lastRefs := 1

				for range time.After(LocalReaderTimeout) {
					refsLk.Lock()
					if refs == 0 && lastRefs == 0 && pf != nil { // pf can't really be nil here, but better be safe
						log.Infow("closing idle partial file", "path", path)
						err := pf.Close()
						if err != nil {
							log.Errorw("closing idle partial file", "path", path, "error", err)
						}

						pf = nil
						refsLk.Unlock()
						return
					}
					lastRefs = refs
					refsLk.Unlock()
				}
			}

			getPF := func() (*partialfile.PartialFile, func() error, error) {
				refsLk.Lock()
				defer refsLk.Unlock()

				if pf == nil {
					// got closed in the meantime, reopen

					var err error
					pf, err = r.pfHandler.OpenPartialFile(abi.PaddedPieceSize(ssize), path)
					if err != nil {
						return nil, nil, xerrors.Errorf("reopening partial file: %w", err)
					}
					log.Debugf("local partial file reopened %s (+%d,%d)", path, offset, size)

					go cleanupIdle()
				}

				refs++

				return pf, func() error {
					refsLk.Lock()
					defer refsLk.Unlock()

					refs--
					return nil
				}, nil
			}

			return func(startOffsetAligned, endOffsetAligned storiface.PaddedByteIndex) (io.ReadCloser, error) {
				pf, done, err := getPF()
				if err != nil {
					return nil, xerrors.Errorf("getting partialfile handle: %w", err)
				}

				r, err := r.pfHandler.Reader(pf, storiface.PaddedByteIndex(offset)+startOffsetAligned, abi.PaddedPieceSize(endOffsetAligned-startOffsetAligned))
				if err != nil {
					return nil, err
				}

				return struct {
					io.Reader
					io.Closer
				}{
					Reader: r,
					Closer: funcCloser(done),
				}, nil
			}, nil

		}

		log.Debugf("miner has unsealed file but not unseal piece, %s (+%d,%d)", path, offset, size)
		if err := r.pfHandler.Close(pf); err != nil {
			return nil, xerrors.Errorf("close partial file: %w", err)
		}
	}

	// --- We don't have the unsealed piece in an unsealed sector file locally

	// if we don't have the unsealed sector file locally, we'll first lookup the Miner Sector Store Index
	// to determine which workers have the unsealed file and then query those workers to know
	// if they have the unsealed piece in the unsealed sector file.
	si, err := r.index.StorageFindSector(ctx, s.ID, ft, 0, false)
	if err != nil {
		log.Debugf("Reader, did not find unsealed file on any of the workers %s (+%d,%d)", path, offset, size)
		return nil, err
	}

	if len(si) == 0 {
		return nil, xerrors.Errorf("failed to read sector %v from remote(%d): %w", s, ft, storiface.ErrSectorNotFound)
	}

	sort.Slice(si, func(i, j int) bool {
		return si[i].Weight > si[j].Weight
	})

	var lastErr error
	for _, info := range si {
		for _, url := range info.URLs {
			// checkAllocated makes a JSON RPC query to a remote worker to determine if it has
			// unsealed piece in their unsealed sector file.
			ok, err := r.checkAllocated(ctx, url, s.ProofType, offset, size)
			if err != nil {
				log.Warnw("check if remote has piece", "url", url, "error", err)
				lastErr = err
				continue
			}
			if !ok {
				continue
			}

			return func(startOffsetAligned, endOffsetAligned storiface.PaddedByteIndex) (io.ReadCloser, error) {
				// readRemote fetches a reader that we can use to read the unsealed piece from the remote worker.
				// It uses a ranged HTTP query to ensure we ONLY read the unsealed piece and not the entire unsealed file.
				rd, err := r.readRemote(ctx, url, offset+abi.PaddedPieceSize(startOffsetAligned), offset+abi.PaddedPieceSize(endOffsetAligned))
				if err != nil {
					log.Warnw("reading from remote", "url", url, "error", err)
					return nil, err
				}

				return rd, err
			}, nil

		}
	}

	// we couldn't find a unsealed file with the unsealed piece, will return a nil reader.
	log.Debugf("returning nil reader, did not find unsealed piece for %+v (+%d,%d), last error=%s", s, offset, size, lastErr)
	return nil, nil
}

// ReaderSeq creates a simple sequential reader for a file. Does not work for
// file types which are a directory (e.g. FTCache).
func (r *Remote) ReaderSeq(ctx context.Context, s storiface.SectorRef, ft storiface.SectorFileType) (io.ReadCloser, error) {
	paths, _, err := r.local.AcquireSector(ctx, s, ft, storiface.FTNone, storiface.PathStorage, storiface.AcquireMove)
	if err != nil {
		return nil, xerrors.Errorf("acquire local: %w", err)
	}

	path := storiface.PathByType(paths, ft)
	if path != "" {
		return os.Open(path)
	}

	si, err := r.index.StorageFindSector(ctx, s.ID, ft, 0, false)
	if err != nil {
		log.Debugf("Reader, did not find file on any of the workers %s (%s)", path, ft.String())
		return nil, err
	}

	if len(si) == 0 {
		return nil, xerrors.Errorf("failed to read sector %v from remote(%d): %w", s, ft, storiface.ErrSectorNotFound)
	}

	sort.Slice(si, func(i, j int) bool {
		return si[i].Weight > si[j].Weight
	})

	for _, info := range si {
		for _, url := range info.URLs {
			rd, err := r.readRemote(ctx, url, 0, 0)
			if err != nil {
				log.Warnw("reading from remote", "url", url, "error", err)
				continue
			}

			return rd, err
		}
	}

	return nil, xerrors.Errorf("failed to read sector %v from remote(%d): %w", s, ft, storiface.ErrSectorNotFound)
}

func (r *Remote) Reserve(ctx context.Context, sid storiface.SectorRef, ft storiface.SectorFileType, storageIDs storiface.SectorPaths, overheadTab map[storiface.SectorFileType]int, minFreePercentage float64) (func(), error) {
	log.Warnf("reserve called on remote store, sectorID: %v", sid.ID)
	return func() {

	}, nil
}

func (r *Remote) GenerateSingleVanillaProof(ctx context.Context, minerID abi.ActorID, sinfo storiface.PostSectorChallenge, ppt abi.RegisteredPoStProof) ([]byte, error) {
	p, err := r.local.GenerateSingleVanillaProof(ctx, minerID, sinfo, ppt)
	if err != errPathNotFound {
		return p, err
	}

	sid := abi.SectorID{
		Miner:  minerID,
		Number: sinfo.SectorNumber,
	}

	ft := storiface.FTSealed | storiface.FTCache
	if sinfo.Update {
		ft = storiface.FTUpdate | storiface.FTUpdateCache
	}

	si, err := r.index.StorageFindSector(ctx, sid, ft, 0, false)
	if err != nil {
		return nil, xerrors.Errorf("finding sector %d failed: %w", sid, err)
	}

	requestParams := SingleVanillaParams{
		Miner:     minerID,
		Sector:    sinfo,
		ProofType: ppt,
	}
	jreq, err := json.Marshal(requestParams)
	if err != nil {
		return nil, err
	}

	merr := xerrors.Errorf("sector not found")

	for _, info := range si {
		for _, u := range info.BaseURLs {
			url := fmt.Sprintf("%s/vanilla/single", u)

			req, err := http.NewRequest("POST", url, strings.NewReader(string(jreq)))
			if err != nil {
				merr = multierror.Append(merr, xerrors.Errorf("request: %w", err))
				log.Warnw("GenerateSingleVanillaProof request failed", "url", url, "error", err)
				continue
			}

			if r.auth != nil {
				req.Header = r.auth.Clone()
			}
			req = req.WithContext(ctx)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				merr = multierror.Append(merr, xerrors.Errorf("do request: %w", err))
				log.Warnw("GenerateSingleVanillaProof do request failed", "url", url, "error", err)
				continue
			}

			if resp.StatusCode != http.StatusOK {
				if resp.StatusCode == http.StatusNotFound {
					log.Debugw("reading vanilla proof from remote not-found response", "url", url, "store", info.ID)
					continue
				}
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					merr = multierror.Append(merr, xerrors.Errorf("resp.Body ReadAll: %w", err))
					log.Warnw("GenerateSingleVanillaProof read response body failed", "url", url, "error", err)
					continue
				}

				if err := resp.Body.Close(); err != nil {
					log.Error("response close: ", err)
				}

				merr = multierror.Append(merr, xerrors.Errorf("non-200 code from %s: '%s'", url, strings.TrimSpace(string(body))))
				log.Warnw("GenerateSingleVanillaProof non-200 code from remote", "code", resp.StatusCode, "url", url, "body", string(body))
				continue
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				if err := resp.Body.Close(); err != nil {
					log.Error("response close: ", err)
				}

				merr = multierror.Append(merr, xerrors.Errorf("resp.Body ReadAll: %w", err))
				log.Warnw("GenerateSingleVanillaProof read response body failed", "url", url, "error", err)
				continue
			}

			_ = resp.Body.Close()

			return body, nil
		}
	}

	return nil, merr
}

func (r *Remote) GeneratePoRepVanillaProof(ctx context.Context, sr storiface.SectorRef, sealed, unsealed cid.Cid, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness) ([]byte, error) {
	// Attempt to generate the proof locally first
	p, err := r.local.GeneratePoRepVanillaProof(ctx, sr, sealed, unsealed, ticket, seed)
	if err != errPathNotFound {
		return p, err
	}

	// Define the file types to look for based on the sector's state
	ft := storiface.FTSealed | storiface.FTCache

	// Find sector information
	si, err := r.index.StorageFindSector(ctx, sr.ID, ft, 0, false)
	if err != nil {
		return nil, xerrors.Errorf("finding sector %d failed: %w", sr.ID, err)
	}

	// Prepare request parameters
	requestParams := PoRepVanillaParams{
		Sector:   sr,
		Sealed:   sealed,
		Unsealed: unsealed,
		Ticket:   ticket,
		Seed:     seed,
	}
	jreq, err := json.Marshal(requestParams)
	if err != nil {
		return nil, err
	}

	merr := xerrors.Errorf("sector not found")

	// Iterate over all found sector locations
	for _, info := range si {
		for _, u := range info.BaseURLs {
			url := fmt.Sprintf("%s/vanilla/porep", u)

			// Create and send the request
			req, err := http.NewRequest("POST", url, strings.NewReader(string(jreq)))
			if err != nil {
				merr = multierror.Append(merr, xerrors.Errorf("request: %w", err))
				log.Warnw("GeneratePoRepVanillaProof request failed", "url", url, "error", err)
				continue
			}

			// Set auth headers if available
			if r.auth != nil {
				req.Header = r.auth.Clone()
			}
			req = req.WithContext(ctx)

			// Execute the request
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				merr = multierror.Append(merr, xerrors.Errorf("do request: %w", err))
				log.Warnw("GeneratePoRepVanillaProof do request failed", "url", url, "error", err)
				continue
			}

			// Handle non-OK status codes
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()

				if resp.StatusCode == http.StatusNotFound {
					log.Debugw("reading vanilla proof from remote not-found response", "url", url, "store", info.ID)
					continue
				}

				merr = multierror.Append(merr, xerrors.Errorf("non-200 code from %s: '%s'", url, strings.TrimSpace(string(body))))
				log.Warnw("GeneratePoRepVanillaProof non-200 code from remote", "code", resp.StatusCode, "url", url, "body", string(body))
				continue
			}

			// Read the response body
			body, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				merr = multierror.Append(merr, xerrors.Errorf("resp.Body ReadAll: %w", err))
				log.Warnw("GeneratePoRepVanillaProof read response body failed", "url", url, "error", err)
				continue
			}

			// Return the proof if successful
			return body, nil
		}
	}

	// Return the accumulated error if the proof was not generated
	return nil, merr
}

type funcCloser func() error

func (f funcCloser) Close() error {
	return f()
}

var _ io.Closer = funcCloser(nil)
