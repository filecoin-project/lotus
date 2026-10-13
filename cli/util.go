package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/fatih/color"
	"github.com/ipfs/go-cid"
	"github.com/mattn/go-isatty"

	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/chain/types"
)

// Set the global default, to be overridden by individual cli flags in order
func init() {
	color.NoColor = os.Getenv("GOLOG_LOG_FMT") != "color" &&
		!isatty.IsTerminal(os.Stdout.Fd()) &&
		!isatty.IsCygwinTerminal(os.Stdout.Fd())
}

func parseTipSet(ctx context.Context, api v0api.FullNode, vals []string) (*types.TipSet, error) {
	var headers []*types.BlockHeader
	for _, c := range vals {
		blkc, err := cid.Decode(c)
		if err != nil {
			return nil, err
		}

		bh, err := api.ChainGetBlock(ctx, blkc)
		if err != nil {
			return nil, err
		}

		headers = append(headers, bh)
	}

	return types.NewTipSet(headers)
}

func PrintJson(obj interface{}) error {
	resJson, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling json: %w", err)
	}

	fmt.Println(string(resJson))
	return nil
}

// escapeControl replaces control characters, including the C1 range that JSON
// encoding leaves as-is, with \uXXXX escapes so chain-sourced strings cannot
// drive the terminal. JSON input stays valid JSON.
func escapeControl(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) {
			_, _ = fmt.Fprintf(&b, "\\u%04x", r)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
