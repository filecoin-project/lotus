package shared

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestRetryStream(t *testing.T) {
	tcases := []struct {
		attempts   int
		errors     int
		expSuccess bool
	}{{
		attempts:   1,
		errors:     0,
		expSuccess: true,
	}, {
		attempts:   1,
		errors:     1,
		expSuccess: false,
	}, {
		attempts:   2,
		errors:     1,
		expSuccess: true,
	}, {
		attempts:   2,
		errors:     2,
		expSuccess: false,
	}}
	for _, tcase := range tcases {
		name := fmt.Sprintf("%d attempts, %d errors", tcase.attempts, tcase.errors)
		t.Run(name, func(t *testing.T) {
			opener := &mockOpener{
				errs: make(chan error, tcase.errors),
			}
			for i := 0; i < tcase.errors; i++ {
				opener.errs <- xerrors.Errorf("network err")
			}
			params := RetryParameters(
				time.Millisecond,
				time.Millisecond,
				float64(tcase.attempts),
				1)
			rs := NewRetryStream(opener, params)
			_, err := rs.OpenStream(context.Background(), peer.ID("peer1"), []protocol.ID{"proto1"})
			if tcase.expSuccess {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

type mockOpener struct {
	errs chan error
}

func (o *mockOpener) NewStream(ctx context.Context, p peer.ID, pids ...protocol.ID) (network.Stream, error) {
	select {
	case e := <-o.errs:
		return nil, e
	default:
		return nil, nil
	}
}
