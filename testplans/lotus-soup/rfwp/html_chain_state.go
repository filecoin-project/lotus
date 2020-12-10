package rfwp

import (
	"context"
	"fmt"
	"os"

	"github.com/filecoin-project/oni/lotus-soup/testkit"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/cli"
	tstats "github.com/filecoin-project/lotus/tools/stats"
	"github.com/ipfs/go-cid"
)

func FetchChainState(t *testkit.TestEnvironment, m *testkit.LotusMiner) error {
	height := 0
	headlag := 3

	ctx := context.Background()
	api := m.FullApi

	tipsetsCh, err := tstats.GetTips(ctx, m.FullApi, abi.ChainEpoch(height), headlag)
	if err != nil {
		return err
	}

	for tipset := range tipsetsCh {
		err := func() error {
			filename := fmt.Sprintf("%s%cchain-state-%d.html", t.TestOutputsPath, os.PathSeparator, tipset.Height())
			file, err := os.Create(filename)
			defer file.Close()
			if err != nil {
				return err
			}

			stout, err := api.StateCompute(ctx, tipset.Height(), nil, tipset.Key())
			if err != nil {
				return err
			}

			codeCache := map[address.Address]cid.Cid{}
			getCode := func(addr address.Address) (cid.Cid, error) {
				if c, found := codeCache[addr]; found {
					return c, nil
				}

				c, err := api.StateGetActor(ctx, addr, tipset.Key())
				if err != nil {
					return cid.Cid{}, err
				}

				codeCache[addr] = c.Code
				return c.Code, nil
			}

			return cli.ComputeStateHTMLTempl(file, tipset, stout, getCode)
		}()
		if err != nil {
			return err
		}
	}

	return nil
}
