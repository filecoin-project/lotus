package stores

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/bits"
	"mime"
	"net/http"
	"net/url"
	"os"
	gopath "path"
	"path/filepath"
	"sort"
	"sync"

	"github.com/filecoin-project/lotus/extern/sector-storage/fsutil"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	"github.com/filecoin-project/lotus/extern/sector-storage/tarutil"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"
)

var FetchTempSubdir = "fetching"

var CopyBuf = 1 << 20

type Remote struct {
	local Store
	index SectorIndex
	auth  http.Header

	limit chan struct{}

	fetchLk  sync.Mutex
	fetching map[abi.SectorID]chan struct{}

	pfHandler PartialFileHandler
}

func (r *Remote) RemoveCopies(ctx context.Context, s abi.SectorID, types storiface.SectorFileType) error {
	// TODO: do this on remotes too
	//  (not that we really need to do that since it's always called by the
	//   worker which pulled the copy)

	return r.local.RemoveCopies(ctx, s, types)
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

func (r *Remote) AcquireSector(ctx context.Context, s storage.SectorRef, existing storiface.SectorFileType, allocate storiface.SectorFileType, pathType storiface.PathType, op storiface.AcquireMode) (storiface.SectorPaths, storiface.SectorPaths, error) {
	if existing|allocate != existing^allocate {
		return storiface.SectorPaths{}, storiface.SectorPaths{}, xerrors.New("can't both find and allocate a sector")
	}

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

	paths, stores, err := r.local.AcquireSector(ctx, s, existing, allocate, pathType, op)
	if err != nil {
		return storiface.SectorPaths{}, storiface.SectorPaths{}, xerrors.Errorf("local acquire error: %w", err)
	}

	var toFetch storiface.SectorFileType
	for _, fileType := range storiface.PathTypes {
		if fileType&existing == 0 {
			continue
		}

		if storiface.PathByType(paths, fileType) == "" {
			toFetch |= fileType
		}
	}

	apaths, ids, err := r.local.AcquireSector(ctx, s, storiface.FTNone, toFetch, pathType, op)
	if err != nil {
		return storiface.SectorPaths{}, storiface.SectorPaths{}, xerrors.Errorf("allocate local sector for fetching: %w", err)
	}

	odt := storiface.FSOverheadSeal
	if pathType == storiface.PathStorage {
		odt = storiface.FsOverheadFinalized
	}

	releaseStorage, err := r.local.Reserve(ctx, s, toFetch, ids, odt)
	if err != nil {
		return storiface.SectorPaths{}, storiface.SectorPaths{}, xerrors.Errorf("reserving storage space: %w", err)
	}
	defer releaseStorage()

	for _, fileType := range storiface.PathTypes {
		if fileType&existing == 0 {
			continue
		}

		if storiface.PathByType(paths, fileType) != "" {
			continue
		}

		dest := storiface.PathByType(apaths, fileType)
		storageID := storiface.PathByType(ids, fileType)

		url, err := r.acquireFromRemote(ctx, s.ID, fileType, dest)
		if err != nil {
			return storiface.SectorPaths{}, storiface.SectorPaths{}, err
		}

		storiface.SetPathByType(&paths, fileType, dest)
		storiface.SetPathByType(&stores, fileType, storageID)

		if err := r.index.StorageDeclareSector(ctx, ID(storageID), s.ID, fileType, op == storiface.AcquireMove); err != nil {
			log.Warnf("declaring sector %v in %s failed: %+v", s, storageID, err)
			continue
		}

		if op == storiface.AcquireMove {
			id := ID(storageID)
			if err := r.deleteFromRemote(ctx, url, &id); err != nil {
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

			err = r.fetch(ctx, url, tempDest)
			if err != nil {
				merr = multierror.Append(merr, xerrors.Errorf("fetch error %s (storage %s) -> %s: %w", url, info.ID, tempDest, err))
				continue
			}

			if err := move(tempDest, dest); err != nil {
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

func (r *Remote) fetch(ctx context.Context, url, outname string) error {
	log.Infof("Fetch %s -> %s", url, outname)

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

	req, err := http.NewRequest("GET", url, nil)
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

	/*bar := pb.New64(w.sizeForType(typ))
	bar.ShowPercent = true
	bar.ShowSpeed = true
	bar.Units = pb.U_BYTES

	barreader := bar.NewProxyReader(resp.Body)

	bar.Start()
	defer bar.Finish()*/

	mediatype, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return xerrors.Errorf("parse media type: %w", err)
	}

	if err := os.RemoveAll(outname); err != nil {
		return xerrors.Errorf("removing dest: %w", err)
	}

	switch mediatype {
	case "application/x-tar":
		return tarutil.ExtractTar(resp.Body, outname)
	case "application/octet-stream":
		f, err := os.Create(outname)
		if err != nil {
			return err
		}
		_, err = io.CopyBuffer(f, resp.Body, make([]byte, CopyBuf))
		if err != nil {
			f.Close() // nolint
			return err
		}
		return f.Close()
	default:
		return xerrors.Errorf("unknown content type: '%s'", mediatype)
	}
}

func (r *Remote) checkAllocated(ctx context.Context, url string, spt abi.RegisteredSealProof, offset, size abi.PaddedPieceSize) (bool, error) {
	url = fmt.Sprintf("%s/%d/allocated/%d/%d", url, spt, offset.Unpadded(), size.Unpadded())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, xerrors.Errorf("request: %w", err)
	}
	req.Header = r.auth.Clone()
	fmt.Printf("req using header: %#v \n", r.auth)
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

func (r *Remote) MoveStorage(ctx context.Context, s storage.SectorRef, types storiface.SectorFileType) error {
	// Make sure we have the data local
	_, _, err := r.AcquireSector(ctx, s, types, storiface.FTNone, storiface.PathStorage, storiface.AcquireMove)
	if err != nil {
		return xerrors.Errorf("acquire src storage (remote): %w", err)
	}

	return r.local.MoveStorage(ctx, s, types)
}

func (r *Remote) Remove(ctx context.Context, sid abi.SectorID, typ storiface.SectorFileType, force bool, keepIn []ID) error {
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
			if err := r.deleteFromRemote(ctx, url, nil); err != nil {
				log.Warnf("remove %s: %+v", url, err)
				continue
			}
			break
		}
	}

	return nil
}

func (r *Remote) deleteFromRemote(ctx context.Context, url string, keepIn *ID) error {
	if keepIn != nil {
		url = url + "?keep=" + string(*keepIn)
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

func (r *Remote) FsStat(ctx context.Context, id ID) (fsutil.FsStat, error) {
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

	rl, err := url.Parse(si.URLs[0])
	if err != nil {
		return fsutil.FsStat{}, xerrors.Errorf("failed to parse url: %w", err)
	}

	rl.Path = gopath.Join(rl.Path, "stat", string(id))

	req, err := http.NewRequest("GET", rl.String(), nil)
	if err != nil {
		return fsutil.FsStat{}, xerrors.Errorf("request: %w", err)
	}
	req.Header = r.auth
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fsutil.FsStat{}, xerrors.Errorf("do request: %w", err)
	}
	switch resp.StatusCode {
	case 200:
		break
	case 404:
		return fsutil.FsStat{}, errPathNotFound
	case 500:
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return fsutil.FsStat{}, xerrors.Errorf("fsstat: got http 500, then failed to read the error: %w", err)
		}

		return fsutil.FsStat{}, xerrors.Errorf("fsstat: got http 500: %s", string(b))
	}

	var out fsutil.FsStat
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fsutil.FsStat{}, xerrors.Errorf("decoding fsstat: %w", err)
	}

	defer resp.Body.Close() // nolint

	return out, nil
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
func (r *Remote) CheckIsUnsealed(ctx context.Context, s storage.SectorRef, offset, size abi.PaddedPieceSize) (bool, error) {
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
func (r *Remote) Reader(ctx context.Context, s storage.SectorRef, offset, size abi.PaddedPieceSize) (io.ReadCloser, error) {
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
			return r.pfHandler.Reader(pf, storiface.PaddedByteIndex(offset), size)
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

			// readRemote fetches a reader that we can use to read the unsealed piece from the remote worker.
			// It uses a ranged HTTP query to ensure we ONLY read the unsealed piece and not the entire unsealed file.
			rd, err := r.readRemote(ctx, url, offset, size)
			if err != nil {
				log.Warnw("reading from remote", "url", url, "error", err)
				lastErr = err
				continue
			}
			log.Infof("Read remote %s (+%d,%d)", url, offset, size)
			return rd, nil
		}
	}

	// we couldn't find a unsealed file with the unsealed piece, will return a nil reader.
	log.Debugf("returning nil reader, did not find unsealed piece for %+v (+%d,%d), last error=%s", s, offset, size, lastErr)
	return nil, nil
}

func (r *Remote) Reserve(ctx context.Context, sid storage.SectorRef, ft storiface.SectorFileType, storageIDs storiface.SectorPaths, overheadTab map[storiface.SectorFileType]int) (func(), error) {
	log.Warnf("reserve called on remote store, sectorID: %v", sid.ID)
	return func() {

	}, nil
}

var _ Store = &Remote{}
