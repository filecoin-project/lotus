package spcli

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-state-types/network"
)

// newCtxWithFlags builds a urfave/cli Context with the given flag definitions
// registered and parsed from the given args. This mirrors the pattern used by
// cli/miner/actor_test.go so that cctx.IsSet reflects flags actually passed.
func newCtxWithFlags(t *testing.T, defs []cli.Flag, args ...string) *cli.Context {
	t.Helper()
	app := cli.NewApp()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	for _, f := range defs {
		require.NoError(t, f.Apply(fs))
	}
	require.NoError(t, fs.Parse(args))
	return cli.NewContext(app, fs, nil)
}

func TestSectorsExtendRejectsOnlyCC(t *testing.T) {
	ctx := newCtxWithFlags(t, []cli.Flag{&cli.BoolFlag{Name: "only-cc"}}, "--only-cc")

	err := rejectDeprecatedOnlyCCFlag(ctx)
	require.ErrorContains(t, err, "only-cc flag has been removed")
}

func TestSectorsExtendRejectsDropClaimsOnNV29(t *testing.T) {
	ctx := newCtxWithFlags(t, []cli.Flag{&cli.BoolFlag{Name: "drop-claims"}}, "--drop-claims")

	err := rejectDeprecatedDropClaimsFlag(ctx, network.Version29)
	require.ErrorContains(t, err, "drop-claims flag has been removed")
}

func TestSectorsExtendAllowsDropClaimsOnNV28(t *testing.T) {
	ctx := newCtxWithFlags(t, []cli.Flag{&cli.BoolFlag{Name: "drop-claims"}}, "--drop-claims")

	err := rejectDeprecatedDropClaimsFlag(ctx, network.Version28)
	require.NoError(t, err)
}
