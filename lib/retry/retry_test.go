package retry

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"
)

func TestRetryErrorIsInTrue(t *testing.T) {
	errorsToRetry := []error{&jsonrpc.RPCConnectionError{}}
	require.True(t, ErrorIsIn(&jsonrpc.RPCConnectionError{}, errorsToRetry))
}

func TestRetryErrorIsInFalse(t *testing.T) {
	errorsToRetry := []error{&jsonrpc.RPCConnectionError{}}
	require.False(t, ErrorIsIn(xerrors.Errorf("random error"), errorsToRetry))
}

func TestRetryWrappedErrorIsInTrue(t *testing.T) {
	errorsToRetry := []error{&jsonrpc.RPCConnectionError{}}
	require.True(t, ErrorIsIn(xerrors.Errorf("wrapped: %w", &jsonrpc.RPCConnectionError{}), errorsToRetry))
}
