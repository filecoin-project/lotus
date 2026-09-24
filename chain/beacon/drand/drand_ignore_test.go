package drand

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
)

func TestIgnoreDrandEntry(t *testing.T) {
	t.Setenv("LOTUS_IGNORE_DRAND", "_yes_")

	todayTs := uint64(1652222222)
	db, err := NewDrandBeacon(todayTs, buildconstants.BlockDelaySecs, nil, buildconstants.DrandConfigs[buildconstants.DrandQuicknet])
	require.NoError(t, err)

	resp := <-db.Entry(context.Background(), 42)
	require.NoError(t, resp.Err)
	assert.Equal(t, uint64(42), resp.Entry.Round)
	assert.Len(t, resp.Entry.Data, drandStubSigLen)
	assert.Equal(t, make([]byte, drandStubSigLen), resp.Entry.Data)
}

func TestIgnoreDrandVerifyEntry(t *testing.T) {
	t.Setenv("LOTUS_IGNORE_DRAND", "_yes_")

	todayTs := uint64(1652222222)
	db, err := NewDrandBeacon(todayTs, buildconstants.BlockDelaySecs, nil, buildconstants.DrandConfigs[buildconstants.DrandQuicknet])
	require.NoError(t, err)

	err = db.VerifyEntry(types.BeaconEntry{Round: 1, Data: []byte("bogus")}, nil)
	assert.NoError(t, err)
}
