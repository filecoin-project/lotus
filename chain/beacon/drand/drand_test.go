package drand

import (
	"os"
	"testing"
	"time"

	dchain "github.com/drand/drand/chain"
	hclient "github.com/drand/drand/client/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

func TestPrintGroupInfo(t *testing.T) {
	server := build.DrandConfig().Servers[0]
	c, err := hclient.New(server, nil, nil)
	assert.NoError(t, err)
	cg := c.(interface {
		FetchChainInfo(groupHash []byte) (*dchain.Info, error)
	})
	chain, err := cg.FetchChainInfo(nil)
	assert.NoError(t, err)
	err = chain.ToJSON(os.Stdout)
	assert.NoError(t, err)
}

func TestDrandBeaconMinDrandEntry(t *testing.T) {
	db := DrandBeacon{
		interval:     30 * time.Second,
		drandGenTime: 666,
		filGenTime:   1337,
		filRoundTime: 30,
	}

	// test for equal frequency
	in := abi.ChainEpoch(10)
	res := db.MinDrandEntryEpoch(in)
	require.Equal(t, in-1, res)

	// test for different frequency
	// since we give an odd epoch number, the drand entry should have been
	// inserted in the even epoch number given filecoin is twice faster than
	// drand
	db.interval = 60 * time.Second
	in = abi.ChainEpoch(11)
	res = db.MinDrandEntryEpoch(in)
	require.Equal(t, in-1, res)
}
