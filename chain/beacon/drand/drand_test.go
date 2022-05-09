//stm: ignore
//Only tests external library behavior, therefore it should not be annotated
package drand

import (
	"os"
	"testing"

	dchain "github.com/drand/drand/chain"
	hclient "github.com/drand/drand/client/http"
	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/lotus/build"
)

func TestPrintGroupInfo(t *testing.T) {
	// beware, the drand server being tested depends on the build configuration
	// of the network. This is guided by the LOTUS_NETWORK environment variable
	schedule := build.DrandConfigSchedule()
	if len(schedule) == 0 {
		t.Fail()
	}
	point := schedule[len(schedule)-1]
	server := point.Config.Servers[0]
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
