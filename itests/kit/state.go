package kit

import (
	"context"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	actorstypes "github.com/filecoin-project/go-state-types/actors"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

// AssertActorType verifies that the supplied address is an actor of the
// specified type (as per its manifest key).
func (f *TestFullNode) AssertActorType(ctx context.Context, addr address.Address, actorType string) {
	// validate that an placeholder was created
	act, err := f.StateGetActor(ctx, addr, types.EmptyTSK)
	require.NoError(f.t, err)

	nv, err := f.StateNetworkVersion(ctx, types.EmptyTSK)
	require.NoError(f.t, err)

	av, err := actorstypes.VersionForNetwork(nv)
	require.NoError(f.t, err)

	codecid, exists := actors.GetActorCodeID(av, actorType)
	require.True(f.t, exists)

	// check the code CID
	require.Equal(f.t, codecid, act.Code)
}
