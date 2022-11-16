package build_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	actorstypes "github.com/filecoin-project/go-state-types/actors"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
)

// Test that the embedded metadata is correct.
func TestEmbeddedMetadata(t *testing.T) {
	metadata, err := build.ReadEmbeddedBuiltinActorsMetadata()
	require.NoError(t, err)

	require.Equal(t, metadata, build.EmbeddedBuiltinActorsMetadata)
}

// Test that we're registering the manifest correctly.
func TestRegistration(t *testing.T) {
	manifestCid, found := actors.GetManifest(actorstypes.Version8)
	require.True(t, found)
	require.True(t, manifestCid.Defined())

	for _, key := range actors.GetBuiltinActorsKeys(actorstypes.Version8) {
		actorCid, found := actors.GetActorCodeID(actorstypes.Version8, key)
		require.True(t, found)
		name, version, found := actors.GetActorMetaByCode(actorCid)
		require.True(t, found)
		require.Equal(t, actorstypes.Version8, version)
		require.Equal(t, key, name)
	}
}
