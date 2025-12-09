//go:build release

package build_test

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car/v2"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"

	actorstypes "github.com/filecoin-project/go-state-types/actors"

	"github.com/filecoin-project/lotus/build"
)

func TestEmbeddedBuiltinActorsMetadata(t *testing.T) {
	subjectsByVersionByNetworks := make(map[actorstypes.Version]map[string]*build.BuiltinActorsMetadata)
	for _, subject := range build.EmbeddedBuiltinActorsMetadata {
		if subject.BundleGitTag == "" {
			// BundleGitTag is required to verify the SHA-256 checksum.
			// The pack script only includes this for the latest network version, and it is good enough to only
			// check the latest network version metadata. Hence the skip.
			continue
		}
		v, ok := subjectsByVersionByNetworks[subject.Version]
		if !ok {
			v = make(map[string]*build.BuiltinActorsMetadata)
		}
		v[subject.Network] = subject
		subjectsByVersionByNetworks[subject.Version] = v
	}

	for version, networks := range subjectsByVersionByNetworks {
		cachedCar, err := os.Open(fmt.Sprintf("./actors/v%v.tar.zst", version))
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, cachedCar.Close()) })
		zstReader, err := zstd.NewReader(cachedCar)
		require.NoError(t, err)
		tarReader := tar.NewReader(zstReader)
		for {
			header, err := tarReader.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err)

			network := strings.TrimSuffix(strings.TrimPrefix(header.Name, "builtin-actors-"), ".car")
			subject, found := networks[network]
			if !found {
				continue
			}

			shaURL := fmt.Sprintf("https://github.com/filecoin-project/builtin-actors/releases/download/%s/builtin-actors-%s.sha256", subject.BundleGitTag, subject.Network)
			resp, err := http.Get(shaURL)
			require.NoError(t, err, "failed to retrieve CAR SHA")
			require.Equal(t, http.StatusOK, resp.StatusCode, "unexpected response status code while retrieving CAR SHA")

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, resp.Body.Close())
			require.NoError(t, err)
			fields := strings.Fields(string(respBody))
			require.Len(t, fields, 2)
			wantShaHex := fields[0]

			hasher := sha256.New()
			reader, err := car.NewBlockReader(io.TeeReader(tarReader, hasher))
			require.NoError(t, err)

			require.EqualValues(t, 1, reader.Version)
			require.Len(t, reader.Roots, 1, "expected exactly one root CID for builtin actors bundle network %s, version %v", subject.Network, subject.Version)
			require.True(t, reader.Roots[0].Equals(subject.ManifestCid), "manifest CID does not match")

			subjectActorsByCid := make(map[cid.Cid]string)
			for name, c := range subject.Actors {
				subjectActorsByCid[c] = name
			}
			for {
				next, err := reader.Next()
				if errors.Is(err, io.EOF) {
					break
				}
				require.NoError(t, err)
				name, found := subjectActorsByCid[next.Cid()]
				if found {
					t.Logf("OK: %sv%v/%s -> %s", subject.Network, subject.Version, name, next.Cid())
					delete(subjectActorsByCid, next.Cid())
				}
			}
			require.Empty(t, subjectActorsByCid, "ZST CAR bundle did not contain CIDs for all actors; missing: %v", subjectActorsByCid)

			gotShaHex := hex.EncodeToString(hasher.Sum(nil))
			require.Equal(t, wantShaHex, gotShaHex, "SHA-256 digest of ZST CAR bundle does not match builtin-actors release")
			delete(networks, network)
		}
		require.Empty(t, networks, "CAR bundle did not contain CIDs for network; missing: %v", networks)
	}
}
