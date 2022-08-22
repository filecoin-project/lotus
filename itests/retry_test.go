package itests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/filecoin-project/lotus/lib/retry"
)

func TestRetryErrorIsInTrue(t *testing.T) {
	errorsToRetry := []error{&jsonrpc.RPCConnectionError{}}
	require.True(t, retry.ErrorIsIn(&jsonrpc.RPCConnectionError{}, errorsToRetry))
}

func TestRetryErrorIsInFalse(t *testing.T) {
	errorsToRetry := []error{&jsonrpc.RPCConnectionError{}}
	require.False(t, retry.ErrorIsIn(xerrors.Errorf("random error"), errorsToRetry))
}
