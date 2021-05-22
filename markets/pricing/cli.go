package pricing

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"golang.org/x/xerrors"
)

func ExternalRetrievalPricingFunc(cmd string) dtypes.RetrievalPricingFunc {
	return func(ctx context.Context, pricingInput retrievalmarket.PricingInput) (retrievalmarket.Ask, error) {
		return runPricingFunc(ctx, cmd, pricingInput)
	}
}

func runPricingFunc(_ context.Context, cmd string, params interface{}) (retrievalmarket.Ask, error) {
	j, err := json.Marshal(params)
	if err != nil {
		return retrievalmarket.Ask{}, err
	}

	var out bytes.Buffer
	var errb bytes.Buffer

	c := exec.Command("sh", "-c", cmd)
	c.Stdin = bytes.NewReader(j)
	c.Stdout = &out
	c.Stderr = &errb

	switch err := c.Run().(type) {
	case nil:
		bz := out.Bytes()
		resp := retrievalmarket.Ask{}

		if err := json.Unmarshal(bz, &resp); err != nil {
			return resp, xerrors.Errorf("failed to parse pricing output %s, err=%w", string(bz), err)
		}
		return resp, nil
	case *exec.ExitError:
		return retrievalmarket.Ask{}, xerrors.Errorf("pricing func exited with error: %s", errb.String())
	default:
		return retrievalmarket.Ask{}, xerrors.Errorf("pricing func cmd run error: %w", err)
	}
}
