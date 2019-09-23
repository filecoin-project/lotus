package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func (ts *testSuite) testMining(t *testing.T) {
	ctx := context.Background()
	apis, _ := ts.makeNodes(t, 1, []int{0})
	api := apis[0]

	h1, err := api.ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(0), h1.Height())

	newHeads, err := api.ChainNotify(ctx)
	require.NoError(t, err)
	<-newHeads

	err = api.MineOne(ctx)
	require.NoError(t, err)

	<-newHeads

	h2, err := api.ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(1), h2.Height())
}
