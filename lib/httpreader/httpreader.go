package httpreader

import (
	"io"
	"net/http"

	"golang.org/x/xerrors"
)

// HttpReader is a reader which will read a http resource with a simple get request.
// Before first Read it will be passed over JsonRPC as a URL.
type HttpReader struct {
	URL string

	reader io.ReadCloser
}

func (h *HttpReader) Close() error {
	h.URL = ""
	if h.reader != nil {
		return h.reader.Close()
	}
	return nil
}

func (h *HttpReader) Read(p []byte) (n int, err error) {
	if h.reader == nil {
		res, err := http.Get(h.URL)
		if err != nil {
			return 0, err
		}
		if res.StatusCode != http.StatusOK {
			return 0, xerrors.Errorf("unexpected http status %d", res.StatusCode)
		}

		// mark the reader as reading
		h.URL = ""
		h.reader = res.Body
	}
	if h.reader == nil {
		return 0, xerrors.Errorf("http reader closed")
	}

	return h.reader.Read(p)
}

var _ io.ReadCloser = &HttpReader{}
