package stores

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"math/bits"
	"mime"
	"net/http"
	"net/url"
	"os"
	gopath "path"
	"sort"
	"sync"

	"github.com/hashicorp/go-multierror"
	files "github.com/ipfs/go-ipfs-files"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/sector-storage/tarutil"
)

type Remote struct {
	local *Local
	index SectorIndex
	auth  http.Header

	fetchLk sync.Mutex
	fetching map[abi.SectorID]chan struct{}
}

func NewRemote(local *Local, index SectorIndex, auth http.Header) *Remote {
	return &Remote{
		local: local,
		index: index,
		auth:  auth,

		fetching: map[abi.SectorID]chan struct{}{},
	}
}

func (r *Remote) AcquireSector(ctx context.Context, s abi.SectorID, existing SectorFileType, allocate SectorFileType, sealing bool) (SectorPaths, SectorPaths, func(), error) {
	if existing|allocate != existing^allocate {
		return SectorPaths{}, SectorPaths{}, nil, xerrors.New("can't both find and allocate a sector")
	}

	for {
		r.fetchLk.Lock()

		c, locked := r.fetching[s]
		if !locked {
			r.fetching[s] = make(chan struct{})
			r.fetchLk.Unlock()
			break
		}

		r.fetchLk.Unlock()

		select {
		case <-c:
			continue
		case <-ctx.Done():
			return SectorPaths{}, SectorPaths{}, nil, ctx.Err()
		}
	}

	defer func() {
		r.fetchLk.Lock()
		close(r.fetching[s])
		delete(r.fetching, s)
		r.fetchLk.Unlock()
	}()

	paths, stores, done, err := r.local.AcquireSector(ctx, s, existing, allocate, sealing)
	if err != nil {
		return SectorPaths{}, SectorPaths{}, nil, xerrors.Errorf("local acquire error: %w", err)
	}

	for _, fileType := range PathTypes {
		if fileType&existing == 0 {
			continue
		}

		if PathByType(paths, fileType) != "" {
			continue
		}

		ap, storageID, url, rdone, err := r.acquireFromRemote(ctx, s, fileType, sealing)
		if err != nil {
			done()
			return SectorPaths{}, SectorPaths{}, nil, err
		}

		done = mergeDone(done, rdone)
		SetPathByType(&paths, fileType, ap)
		SetPathByType(&stores, fileType, string(storageID))

		if err := r.index.StorageDeclareSector(ctx, storageID, s, fileType); err != nil {
			log.Warnf("declaring sector %v in %s failed: %+v", s, storageID, err)
			continue
		}

		// TODO: some way to allow having duplicated sectors in the system for perf
		if err := r.deleteFromRemote(ctx, url); err != nil {
			log.Warnf("deleting sector %v from %s (delete %s): %+v", s, storageID, url, err)
		}
	}

	return paths, stores, done, nil
}

func (r *Remote) acquireFromRemote(ctx context.Context, s abi.SectorID, fileType SectorFileType, sealing bool) (string, ID, string, func(), error) {
	si, err := r.index.StorageFindSector(ctx, s, fileType, false)
	if err != nil {
		return "", "", "", nil, err
	}

	if len(si) == 0 {
		return "", "", "", nil, xerrors.Errorf("failed to acquire sector %v from remote(%d): not found", s, fileType)
	}

		sort.Slice(si, func(i, j int) bool {
		return si[i].Weight < si[j].Weight
	})

	apaths, ids, done, err := r.local.AcquireSector(ctx, s, FTNone, fileType, sealing)
	if err != nil {
		return "", "", "", nil, xerrors.Errorf("allocate local sector for fetching: %w", err)
	}
	dest := PathByType(apaths, fileType)
	storageID := PathByType(ids, fileType)

	var merr error
	for _, info := range si {
		// TODO: see what we have local, prefer that

		for _, url := range info.URLs {
			err := r.fetch(ctx, url, dest)
			if err != nil {
				merr = multierror.Append(merr, xerrors.Errorf("fetch error %s (storage %s) -> %s: %w", url, info.ID, dest, err))
				continue
			}

			if merr != nil {
				log.Warnw("acquireFromRemote encountered errors when fetching sector from remote", "errors", merr)
			}
			return dest, ID(storageID), url, done, nil
		}
	}

	done()
	return "", "", "", nil, xerrors.Errorf("failed to acquire sector %v from remote (tried %v): %w", s, si, merr)
}

func (r *Remote) fetch(ctx context.Context, url, outname string) error {
	log.Infof("Fetch %s -> %s", url, outname)

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
	defer resp.Body.Close()

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
		return files.WriteTo(files.NewReaderFile(resp.Body), outname)
	default:
		return xerrors.Errorf("unknown content type: '%s'", mediatype)
	}
}

func (r *Remote) MoveStorage(ctx context.Context, s abi.SectorID, types SectorFileType) error {
	// Make sure we have the data local
	_, _, ddone, err := r.AcquireSector(ctx, s, types, FTNone, false)
	if err != nil {
		return xerrors.Errorf("acquire src storage (remote): %w", err)
	}
	ddone()

	return r.local.MoveStorage(ctx, s, types)
}

func (r *Remote) Remove(ctx context.Context, sid abi.SectorID, typ SectorFileType) error {
	if bits.OnesCount(uint(typ)) != 1 {
		return xerrors.New("delete expects one file type")
	}

	if err := r.local.Remove(ctx, sid, typ); err != nil {
		return xerrors.Errorf("remove from local: %w", err)
	}

	si, err := r.index.StorageFindSector(ctx, sid, typ, false)
	if err != nil {
		return xerrors.Errorf("finding existing sector %d(t:%d) failed: %w", sid, typ, err)
	}

	for _, info := range si {
		for _, url := range info.URLs {
			if err := r.deleteFromRemote(ctx, url); err != nil {
				log.Warnf("remove %s: %+v", url, err)
				continue
			}
			break
		}
	}

	return nil
}

func (r *Remote) deleteFromRemote(ctx context.Context, url string) error {
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
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return xerrors.Errorf("non-200 code: %d", resp.StatusCode)
	}

	return nil
}

func (r *Remote) FsStat(ctx context.Context, id ID) (FsStat, error) {
	st, err := r.local.FsStat(ctx, id)
	switch err {
	case nil:
		return st, nil
	case errPathNotFound:
		break
	default:
		return FsStat{}, xerrors.Errorf("local stat: %w", err)
	}

	si, err := r.index.StorageInfo(ctx, id)
	if err != nil {
		return FsStat{}, xerrors.Errorf("getting remote storage info: %w", err)
	}

	if len(si.URLs) == 0 {
		return FsStat{}, xerrors.Errorf("no known URLs for remote storage %s", id)
	}

	rl, err := url.Parse(si.URLs[0])
	if err != nil {
		return FsStat{}, xerrors.Errorf("failed to parse url: %w", err)
	}

	rl.Path = gopath.Join(rl.Path, "stat", string(id))

	req, err := http.NewRequest("GET", rl.String(), nil)
	if err != nil {
		return FsStat{}, xerrors.Errorf("request: %w", err)
	}
	req.Header = r.auth
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return FsStat{}, xerrors.Errorf("do request: %w", err)
	}
	switch resp.StatusCode {
	case 200:
		break
	case 404:
		return FsStat{}, errPathNotFound
	case 500:
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return FsStat{}, xerrors.Errorf("fsstat: got http 500, then failed to read the error: %w", err)
		}

		return FsStat{}, xerrors.Errorf("fsstat: got http 500: %s", string(b))
	}

	var out FsStat
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return FsStat{}, xerrors.Errorf("decoding fsstat: %w", err)
	}

	defer resp.Body.Close()

	return out, nil
}

func mergeDone(a func(), b func()) func() {
	return func() {
		a()
		b()
	}
}

var _ Store = &Remote{}
