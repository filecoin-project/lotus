package node

import (
	"context"
	"github.com/filecoin-project/go-lotus/api/test"
	"testing"

	"github.com/filecoin-project/go-lotus/api"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
)

func builder(t *testing.T, n int) []api.API {
	ctx := context.Background()
	mn := mocknet.New(ctx)

	out := make([]api.API, n)

	for i := 0; i < n; i++ {
		var err error
		out[i], err = New(ctx,
			Online(),
			MockHost(mn),
		)
		if err != nil {
			t.Fatal(err)
		}
	}

	return out
}

func TestAPI(t *testing.T) {
	test.TestApis(t, builder)
}
