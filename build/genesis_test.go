package build_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/ipld/go-car"
	"github.com/klauspost/compress/zstd"
	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/bundle"
)

func TestGenesis(t *testing.T) {
	for _, test := range []struct {
		path    string
		network string
		bundled bool
	}{
		{path: "genesis/butterflynet.car.zst", network: "butterflynet", bundled: true},
		{path: "genesis/calibnet.car.zst", network: "calibrationnet"},
		{path: "genesis/interopnet.car.zst", network: "caterpillarnet"},
		{path: "genesis/mainnet.car.zst", network: "mainnet"},
	} {
		t.Run(test.path, func(t *testing.T) {
			subject, err := os.Open(test.path)
			require.NoError(t, err)

			gotIsCompressed, err := build.IsZstdCompressed(subject)
			require.NoError(t, err)
			require.True(t, gotIsCompressed)

			gotDecompressed, err := build.DecompressAsZstd(subject)
			require.NoError(t, err)
			require.NotEmpty(t, gotDecompressed)

			gotIsCompressed, err = build.IsZstdCompressed(bytes.NewReader(gotDecompressed))
			require.NoError(t, err)
			require.False(t, gotIsCompressed)

			requireGenesisCodeLoadable(t, gotDecompressed, test.network, test.bundled)
		})
	}
}

// requireGenesisCodeLoadable loads a genesis car with its actor code removed, as a car exported
// without the wasm would be, and asserts that the startup bundle load restores the code of every
// actor in the genesis state.
func requireGenesisCodeLoadable(t *testing.T, genesisCar []byte, network string, bundled bool) {
	ctx := context.Background()

	original := buildconstants.NetworkBundle
	require.NoError(t, build.UseNetworkBundle(network))
	t.Cleanup(func() { require.NoError(t, build.UseNetworkBundle(original)) })

	bs := blockstore.NewMemory()
	header, err := car.LoadCar(ctx, bs, bytes.NewReader(genesisCar))
	require.NoError(t, err)
	require.Len(t, header.Roots, 1)

	root, err := bs.Get(ctx, header.Roots[0])
	require.NoError(t, err)
	genesis, err := types.DecodeBlock(root.RawData())
	require.NoError(t, err)

	codes := genesisActorCode(t, bs, genesis)
	for c := range codes {
		require.NoError(t, bs.DeleteBlock(ctx, c))
	}
	require.Equal(t, bundled, len(codes) > 0, "genesis actor code in bundle")

	require.NoError(t, bundle.LoadGenesisBundle(ctx, bs, genesis))

	for c := range codes {
		has, err := bs.Has(ctx, c)
		require.NoError(t, err)
		require.True(t, has, "actor code %s missing from blockstore", c)
	}
}

// genesisActorCode returns the non-identity code CIDs of the actors in the genesis state.
func genesisActorCode(t *testing.T, bs blockstore.Blockstore, genesis *types.BlockHeader) map[cid.Cid]struct{} {
	st, err := state.LoadStateTree(cbor.NewCborStore(bs), genesis.ParentStateRoot)
	require.NoError(t, err)

	codes := make(map[cid.Cid]struct{})
	require.NoError(t, st.ForEach(func(_ address.Address, act *types.Actor) error {
		if act.Code.Prefix().MhType != multihash.IDENTITY {
			codes[act.Code] = struct{}{}
		}
		return nil
	}))
	return codes
}

func TestGenesis_ZstdCheck(t *testing.T) {
	for _, test := range []struct {
		name           string
		given          func(t *testing.T) io.ReadSeeker
		wantCompressed bool
		wantErr        bool
	}{
		{
			name: "arbitraryLongEnough",
			given: func(t *testing.T) io.ReadSeeker {
				return bytes.NewReader([]byte("fish"))
			},
		},
		{
			name: "arbitraryShort",
			given: func(t *testing.T) io.ReadSeeker {
				return bytes.NewReader([]byte("🐠"))
			},
		},
		{
			name: "arbitraryZstdCompressed",
			given: func(t *testing.T) io.ReadSeeker {
				var buf bytes.Buffer
				writer, err := zstd.NewWriter(&buf)
				require.NoError(t, err)
				written, err := writer.Write([]byte("fish"))
				require.NoError(t, err)
				require.NotZero(t, written)
				require.NoError(t, writer.Close())
				return bytes.NewReader(buf.Bytes())
			},
			wantCompressed: true,
		},
		{
			name:    "failingPositionReset",
			given:   func(t *testing.T) io.ReadSeeker { return failOnSeekStart{} },
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			target := test.given(t)
			gotCompressed, gotErr := build.IsZstdCompressed(target)
			require.Equal(t, test.wantCompressed, gotCompressed)
			require.Equal(t, test.wantErr, gotErr != nil)

			if !test.wantErr {
				gotPosition, err := target.Seek(0, io.SeekCurrent)
				require.NoError(t, err)
				require.Zero(t, gotPosition)
			}
		})
	}
}

var _ io.ReadSeeker = (*failOnSeekStart)(nil)

type failOnSeekStart struct{}

func (failOnSeekStart) Read([]byte) (int, error) { return 0, nil }

func (failOnSeekStart) Seek(_ int64, whence int) (int64, error) {
	if whence == io.SeekStart {
		return 0, errors.New("pursue the horizon; forsake the dawn; the start is long gone")
	}
	return 0, nil
}
