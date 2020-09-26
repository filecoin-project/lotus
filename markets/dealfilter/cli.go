package dealfilter

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"

	"github.com/filecoin-project/go-fil-markets/storagemarket"

	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

func CliDealFilter(cmd string) dtypes.DealFilter {
	// TODO: run some checks on the cmd string

	return func(ctx context.Context, deal storagemarket.MinerDeal) (bool, string, error) {
		j, err := json.MarshalIndent(deal, "", "  ")
		if err != nil {
			return false, "", err
		}

		var out bytes.Buffer

		c := exec.Command("sh", "-c", cmd)
		c.Stdin = bytes.NewReader(j)
		c.Stdout = &out
		c.Stderr = &out

		switch err := c.Run().(type) {
		case nil:
			return true, "", nil
		case *exec.ExitError:
			return false, out.String(), nil
		default:
			return false, "filter cmd run error", err
		}

	}
}
