package build_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
)

// Test that the embedded metadata is correct.
func TestEmbeddedMetadata(t *testing.T) {
	metadata, err := build.ReadEmbeddedBuiltinActorsMetadata()
	require.NoError(t, err)

	for i, v1 := range metadata {
		v2 := build.EmbeddedBuiltinActorsMetadata[i]
		require.Equal(t, v1.Network, v2.Network)
		require.Equal(t, v1.Version, v2.Version)
		require.Equal(t, v1.ManifestCid, v2.ManifestCid)
		require.Equal(t, v1.Actors, v2.Actors)
	}
}

// Test that we're registering the manifest correctly.
func TestRegistration(t *testing.T) {
	for _, av := range []actorstypes.Version{actorstypes.Version8, actorstypes.Version9} {
		manifestCid, found := actors.GetManifest(av)
		require.True(t, found)
		require.True(t, manifestCid.Defined())

		for _, key := range manifest.GetBuiltinActorsKeys(av) {
			actorCid, found := actors.GetActorCodeID(av, key)
			require.True(t, found)
			name, version, found := actors.GetActorMetaByCode(actorCid)
			require.True(t, found)
			require.Equal(t, av, version)
			require.Equal(t, key, name)
		}
	}
}
