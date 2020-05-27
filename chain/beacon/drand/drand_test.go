package drand

import (
	"os"
	"testing"

	dchain "github.com/drand/drand/chain"
	dclient "github.com/drand/drand/client"
	"github.com/stretchr/testify/assert"
)

func TestPrintGroupInfo(t *testing.T) {
	c, err := dclient.NewHTTPClient(drandServers[0], nil, nil)
	assert.NoError(t, err)
	cg := c.(interface {
		FetchChainInfo(groupHash []byte) (*dchain.Info, error)
	})
	chain, err := cg.FetchChainInfo(nil)
	assert.NoError(t, err)
	err = chain.ToJSON(os.Stdout)
	assert.NoError(t, err)
}
