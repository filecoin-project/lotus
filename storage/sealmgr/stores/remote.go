package stores

import (
	"context"
	"mime"
	"net/http"
	"os"
	"sort"
	"sync"

	"github.com/hashicorp/go-multierror"
	files "github.com/ipfs/go-ipfs-files"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/lib/tarutil"
	"github.com/filecoin-project/lotus/storage/sealmgr/sectorutil"
)

type Remote struct {
	local *Local
	index SectorIndex
	auth  http.Header

	fetchLk sync.Mutex // TODO: this can be much smarter
	// TODO: allow multiple parallel fetches
	//  (make sure to not fetch the same sector data twice)
}

func NewRemote(local *Local, index SectorIndex, auth http.Header) *Remote {
	return &Remote{
		local: local,
		index: index,
		auth:  auth,
	}
}

func (r *Remote) AcquireSector(ctx context.Context, s abi.SectorID, existing sectorbuilder.SectorFileType, allocate sectorbuilder.SectorFileType, sealing bool) (sectorbuilder.SectorPaths, sectorbuilder.SectorPaths, func(), error) {
	if existing|allocate != existing^allocate {
		return sectorbuilder.SectorPaths{}, sectorbuilder.SectorPaths{}, nil, xerrors.New("can't both find and allocate a sector")
	}

	r.fetchLk.Lock()
	defer r.fetchLk.Unlock()

	paths, stores, done, err := r.local.AcquireSector(ctx, s, existing, allocate, sealing)
	if err != nil {
		return sectorbuilder.SectorPaths{}, sectorbuilder.SectorPaths{}, nil, xerrors.Errorf("local acquire error: %w", err)
	}

	for _, fileType := range pathTypes {
		if fileType&existing == 0 {
			continue
		}

		if sectorutil.PathByType(paths, fileType) != "" {
			continue
		}

		ap, storageID, rdone, err := r.acquireFromRemote(ctx, s, fileType, sealing)
		if err != nil {
			done()
			return sectorbuilder.SectorPaths{}, sectorbuilder.SectorPaths{}, nil, err
		}

		done = mergeDone(done, rdone)
		sectorutil.SetPathByType(&paths, fileType, ap)
		sectorutil.SetPathByType(&stores, fileType, string(storageID))

		if err := r.index.StorageDeclareSector(ctx, storageID, s, fileType); err != nil {
			log.Warnf("declaring sector %v in %s failed: %+v", s, storageID, err)
		}
	}

	return paths, stores, done, nil
}

func (r *Remote) acquireFromRemote(ctx context.Context, s abi.SectorID, fileType sectorbuilder.SectorFileType, sealing bool) (string, ID, func(), error) {
	si, err := r.index.StorageFindSector(ctx, s, fileType)
	if err != nil {
		return "", "", nil, err
	}

	sort.Slice(si, func(i, j int) bool {
		return si[i].Weight < si[j].Weight
	})

	apaths, ids, done, err := r.local.AcquireSector(ctx, s, 0, fileType, sealing)
	if err != nil {
		return "", "", nil, xerrors.Errorf("allocate local sector for fetching: %w", err)
	}
	dest := sectorutil.PathByType(apaths, fileType)
	storageID := sectorutil.PathByType(ids, fileType)

	var merr error
	for _, info := range si {
		for _, url := range info.URLs {
			err := r.fetch(url, dest)
			if err != nil {
				merr = multierror.Append(merr, xerrors.Errorf("fetch error %s (storage %s) -> %s: %w", url, info.ID, dest, err))
				continue
			}

			if merr != nil {
				log.Warnw("acquireFromRemote encountered errors when fetching sector from remote", "errors", merr)
			}
			return dest, ID(storageID), done, nil
		}
	}

	done()
	return "", "", nil, xerrors.Errorf("failed to acquire sector %v from remote (tried %v): %w", s, si, merr)
}

func (r *Remote) fetch(url, outname string) error {
	log.Infof("Fetch %s -> %s", url, outname)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return xerrors.Errorf("request: %w", err)
	}
	req.Header = r.auth

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

func mergeDone(a func(), b func()) func() {
	return func() {
		a()
		b()
	}
}

var _ Store = &Remote{}
