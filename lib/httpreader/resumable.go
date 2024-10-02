package httpreader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"

	logging "github.com/ipfs/go-log/v2"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"
)

var log = logging.Logger("httpreader")

type ResumableReader struct {
	ctx           context.Context
	initialURL    string
	finalURL      *string
	position      int64
	contentLength int64
	client        *http.Client
	reader        io.ReadCloser
}

var _ io.ReadCloser = (*ResumableReader)(nil)

func NewResumableReader(ctx context.Context, url string) (*ResumableReader, error) {
	finalURL := ""

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			finalURL = req.URL.String()
			if len(via) >= 10 {
				return xerrors.New("stopped after 10 redirects")
			}
			return nil
		},
	}

	r := &ResumableReader{
		ctx:        ctx,
		initialURL: url,
		finalURL:   &finalURL,
		position:   0,
		client:     client,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch resource, status code: %d", resp.StatusCode)
	}

	contentLength, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		if err = resp.Body.Close(); err != nil {
			err = multierr.Append(err, err)
		}
		return nil, err
	}

	r.contentLength = contentLength
	r.reader = resp.Body

	return r, nil
}

func (r *ResumableReader) ContentLength() int64 {
	return r.contentLength
}

func (r *ResumableReader) Close() error {
	if r.reader != nil {
		return r.reader.Close()
	}
	return nil
}

func (r *ResumableReader) Read(p []byte) (n int, err error) {
	for {
		if r.reader == nil {
			reqURL := r.initialURL
			if *r.finalURL != "" {
				reqURL = *r.finalURL
			}

			req, err := http.NewRequestWithContext(r.ctx, "GET", reqURL, nil)
			if err != nil {
				return 0, err
			}
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", r.position))
			resp, err := r.client.Do(req)
			if err != nil {
				return 0, err
			}

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
				return 0, fmt.Errorf("non-resumable status code: %d", resp.StatusCode)
			}
			r.reader = resp.Body
		}

		n, err = r.reader.Read(p)
		r.position += int64(n)

		if err == io.EOF || err == io.ErrUnexpectedEOF {
			if r.position == r.contentLength {
				if err := r.reader.Close(); err != nil {
					log.Warnf("error closing reader: %+v", err)
				}
				return n, io.EOF
			}
			if err := r.reader.Close(); err != nil {
				log.Warnf("error closing reader: %+v", err)
			}
			r.reader = nil
		} else {
			return n, err
		}
	}
}
