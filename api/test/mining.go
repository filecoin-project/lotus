package test

import (
	"context"
	"testing"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/stretchr/testify/require"
)

func (ts *testSuite) testMining(t *testing.T) {
	ctx := context.Background()
	apis, sn := ts.makeNodes(t, 1, []int{0})
	api := apis[0]

	h1, err := api.ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, abi.ChainEpoch(0), h1.Height())

	newHeads, err := api.ChainNotify(ctx)
	require.NoError(t, err)
	<-newHeads

	err = sn[0].MineOne(ctx)
	require.NoError(t, err)

	<-newHeads

	h2, err := api.ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, abi.ChainEpoch(1), h2.Height())
}
