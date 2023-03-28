package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/manifest"
	"github.com/filecoin-project/go-state-types/network"
	gstStore "github.com/filecoin-project/go-state-types/store"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors"
	builtin2 "github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/impl"
)

func TestMigrationNV18(t *testing.T) {
	kit.QuietMiningLogs()

	nv18epoch := abi.ChainEpoch(100)
	testClient, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(),
		kit.UpgradeSchedule(stmgr.Upgrade{
			Network: network.Version17,
			Height:  -1,
		}, stmgr.Upgrade{
			Network:   network.Version18,
			Height:    nv18epoch,
			Migration: filcns.UpgradeActorsV10,
		},
		))

	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	clientApi := testClient.FullNode.(*impl.FullNodeAPI)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testClient.WaitTillChain(ctx, kit.HeightAtLeast(nv18epoch+5))

	// Now that we have upgraded, we need to:
	// - the EAM exists, has "empty" state
	// - the EthZeroAddress exists
	// - all actors have nil Address fields

	bs := blockstore.NewAPIBlockstore(testClient)
	ctxStore := gstStore.WrapBlockStore(ctx, bs)

	currTs, err := clientApi.ChainHead(ctx)
	require.NoError(t, err)

	newStateTree, err := state.LoadStateTree(ctxStore, currTs.Blocks()[0].ParentStateRoot)
	require.NoError(t, err)

	require.Equal(t, types.StateTreeVersion5, newStateTree.Version())

	codeIDsv10, ok := actors.GetActorCodeIDsFromManifest(actorstypes.Version10)
	require.True(t, ok)

	// check the EAM actor
	EAMActor, err := newStateTree.GetActor(builtin.EthereumAddressManagerActorAddr)
	require.NoError(t, err)
	require.Equal(t, vm.EmptyObjectCid, EAMActor.Head)
	EAMCodeID, ok := codeIDsv10[manifest.EamKey]
	require.True(t, ok)
	require.Equal(t, EAMCodeID, EAMActor.Code)

	// check the EthZeroAddress
	ethZeroAddr, err := (ethtypes.EthAddress{}).ToFilecoinAddress()
	require.NoError(t, err)
	ethZeroAddrID, err := newStateTree.LookupID(ethZeroAddr)
	require.NoError(t, err)
	ethZeroActor, err := newStateTree.GetActor(ethZeroAddrID)
	require.NoError(t, err)
	require.True(t, builtin2.IsEthAccountActor(ethZeroActor.Code))
	require.Equal(t, vm.EmptyObjectCid, ethZeroActor.Head)

	// check all actor's Address fields
	require.NoError(t, newStateTree.ForEach(func(address address.Address, actor *types.Actor) error {
		if address != ethZeroAddrID {
			require.Nil(t, actor.Address)
		}
		return nil
	}))
}
