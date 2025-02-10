package build_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/build"
)

func TestGenesis(t *testing.T) {
	for _, test := range []struct {
		path string
	}{
		{path: "genesis/butterflynet.car.zst"},
		{path: "genesis/calibnet.car.zst"},
		{path: "genesis/interopnet.car.zst"},
		{path: "genesis/mainnet.car.zst"},
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
		})
	}
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
