package paths

import (
	"context"
	"io"
	"mime"
	"net/http"
	"os"
	"time"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/storage/sealer/tarutil"
)

func fetch(ctx context.Context, url, outname string, header http.Header) (rerr error) {
	log.Infof("Fetch %s -> %s", url, outname)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return xerrors.Errorf("request: %w", err)
	}
	req.Header = header
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return xerrors.Errorf("do request: %w", err)
	}
	defer resp.Body.Close() // nolint

	if resp.StatusCode != 200 {
		return xerrors.Errorf("non-200 code: %d", resp.StatusCode)
	}

	start := time.Now()
	var bytes int64
	defer func() {
		took := time.Since(start)
		mibps := float64(bytes) / 1024 / 1024 * float64(time.Second) / float64(took)
		log.Infow("Fetch done", "url", url, "out", outname, "took", took.Round(time.Millisecond), "bytes", bytes, "MiB/s", mibps, "err", rerr)
	}()

	mediatype, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return xerrors.Errorf("parse media type: %w", err)
	}

	if err := os.RemoveAll(outname); err != nil {
		return xerrors.Errorf("removing dest: %w", err)
	}

	switch mediatype {
	case "application/x-tar":
		bytes, err = tarutil.ExtractTar(resp.Body, outname, make([]byte, CopyBuf))
		return err
	case "application/octet-stream":
		f, err := os.Create(outname)
		if err != nil {
			return err
		}
		bytes, err = io.CopyBuffer(f, resp.Body, make([]byte, CopyBuf))
		if err != nil {
			f.Close() // nolint
			return err
		}
		return f.Close()
	default:
		return xerrors.Errorf("unknown content type: '%s'", mediatype)
	}
}

// FetchWithTemp fetches data into a temp 'fetching' directory, then moves the file to destination
// The set of URLs must refer to the same object, if one fails, another one will be tried.
func FetchWithTemp(ctx context.Context, urls []string, dest string, header http.Header) (string, error) {
	var merr error
	for _, url := range urls {
		tempDest, err := tempFetchDest(dest, true)
		if err != nil {
			return "", err
		}

		if err := os.RemoveAll(dest); err != nil {
			return "", xerrors.Errorf("removing dest: %w", err)
		}

		err = fetch(ctx, url, tempDest, header)
		if err != nil {
			merr = multierror.Append(merr, xerrors.Errorf("fetch error %s -> %s: %w", url, tempDest, err))
			continue
		}

		if err := Move(tempDest, dest); err != nil {
			return "", xerrors.Errorf("fetch move error %s -> %s: %w", tempDest, dest, err)
		}

		if merr != nil {
			log.Warnw("acquireFromRemote encountered errors when fetching sector from remote", "errors", merr)
		}
		return url, nil
	}

	return "", xerrors.Errorf("failed to fetch sector file (tried %v): %w", urls, merr)
}
