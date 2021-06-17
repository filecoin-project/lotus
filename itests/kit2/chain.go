package kit2

import (
	"context"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/stretchr/testify/require"
)

func WaitTillChainHeight(ctx context.Context, t *testing.T, node *TestFullNode, blocktime time.Duration, height int) abi.ChainEpoch {
	for {
		h, err := node.ChainHead(ctx)
		require.NoError(t, err)
		if h.Height() > abi.ChainEpoch(height) {
			return h.Height()
		}
		time.Sleep(blocktime)
	}
}
