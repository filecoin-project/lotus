package httpreader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewResumableReaderMissingContentLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("response"))
	}))
	defer server.Close()

	reader, err := NewResumableReader(context.Background(), server.URL)

	require.Nil(t, reader)
	require.ErrorContains(t, err, "invalid syntax")
}
