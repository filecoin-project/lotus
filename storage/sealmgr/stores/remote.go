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

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/tarutil"
	"github.com/filecoin-project/lotus/storage/sealmgr/sectorutil"
)

type Remote struct {
	local Store
	remote SectorIndex
	auth http.Header

	fetchLk sync.Mutex // TODO: this can be much smarter
	// TODO: allow multiple parallel fetches
	//  (make sure to not fetch the same sector data twice)
}

func NewRemote(local Store, remote SectorIndex, auth http.Header) *Remote {
	return &Remote{
		local:   local,
		remote:  remote,
		auth:    auth,
	}
}

type SectorIndex interface {
	FindSector(context.Context, abi.SectorID, sectorbuilder.SectorFileType) ([]api.StorageInfo, error)
}

func (r *Remote) AcquireSector(ctx context.Context, s abi.SectorID, existing sectorbuilder.SectorFileType, allocate sectorbuilder.SectorFileType, sealing bool) (sectorbuilder.SectorPaths, func(), error) {
	if existing|allocate != existing^allocate {
		return sectorbuilder.SectorPaths{}, nil, xerrors.New("can't both find and allocate a sector")
	}

	r.fetchLk.Lock()
	defer r.fetchLk.Unlock()

	paths, done, err := r.local.AcquireSector(ctx, s, existing, allocate, sealing)
	if err != nil {
		return sectorbuilder.SectorPaths{}, nil, err
	}

	for _, fileType := range pathTypes {
		if fileType&existing == 0 {
			continue
		}

		if sectorutil.PathByType(paths, fileType) != "" {
			continue
		}

		ap, rdone, err := r.acquireFromRemote(ctx, s, fileType, sealing)
		if err != nil {
			done()
			return sectorbuilder.SectorPaths{}, nil, err
		}

		done = mergeDone(done, rdone)
		sectorutil.SetPathByType(&paths, fileType, ap)

	}

	return paths, done, nil
}

func (r *Remote) acquireFromRemote(ctx context.Context, s abi.SectorID, fileType sectorbuilder.SectorFileType, sealing bool) (string, func(), error) {
	si, err := r.remote.FindSector(ctx, s, fileType)
	if err != nil {
		return "", nil, err
	}

	sort.Slice(si, func(i, j int) bool {
		return si[i].Cost < si[j].Cost
	})

	apaths, done, err := r.local.AcquireSector(ctx, s, 0, fileType, sealing)
	if err != nil {
		return "", nil, xerrors.Errorf("allocate local sector for fetching: %w", err)
	}
	dest := sectorutil.PathByType(apaths, fileType)

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
			return dest, done, nil
		}
	}

	done()
	return "", nil, xerrors.Errorf("failed to acquire sector %v from remote: %w", s, merr)
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
