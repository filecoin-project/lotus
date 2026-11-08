package httpreader

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type closeTrackingBody struct {
	io.Reader
	closed bool
}

func (b *closeTrackingBody) Close() error {
	b.closed = true
	return nil
}

type statusTransport struct{ body *closeTrackingBody }

func (t statusTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusNotFound, Body: t.body, Header: make(http.Header)}, nil
}

func TestHttpReaderClosesUnexpectedStatusBody(t *testing.T) {
	body := &closeTrackingBody{Reader: strings.NewReader("not found")}
	previous := http.DefaultTransport
	http.DefaultTransport = statusTransport{body: body}
	t.Cleanup(func() { http.DefaultTransport = previous })

	reader := &HttpReader{URL: "http://example.invalid/resource"}
	_, err := reader.Read(make([]byte, 1))
	require.ErrorContains(t, err, "unexpected http status 404")
	require.True(t, body.closed, "an unsuccessful response body must be closed")
}
