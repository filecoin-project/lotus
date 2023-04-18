package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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

func TestMigrationNV21(t *testing.T) {
	kit.QuietMiningLogs()

	upgradeEpoch := abi.ChainEpoch(100)
	testClient, testMiner, ens := kit.EnsembleMinimal(t, kit.MockProofs(),
		kit.UpgradeSchedule(stmgr.Upgrade{
			Network: network.Version20,
			Height:  -1,
		}, stmgr.Upgrade{
			Network:   network.Version21,
			Height:    upgradeEpoch,
			Migration: filcns.UpgradeActorsV13,
		},
		))

	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	clientApi := testClient.FullNode.(*impl.FullNodeAPI)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testClient.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))

	bs := blockstore.NewAPIBlockstore(testClient)
	ctxStore := gstStore.WrapBlockStore(ctx, bs)

	currTs, err := clientApi.ChainHead(ctx)
	require.NoError(t, err)

	// Verify network version.
	nv, err := testClient.StateNetworkVersion(ctx, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, network.Version21, nv)

	newStateTree, err := state.LoadStateTree(ctxStore, currTs.Blocks()[0].ParentStateRoot)
	require.NoError(t, err)

	require.Equal(t, types.StateTreeVersion5, newStateTree.Version())

	codeCids1, ok := actors.GetActorCodeIDsFromManifest(actorstypes.Version13)
	require.True(t, ok)

	codeCids2, err := clientApi.StateActorCodeCIDs(ctx, nv)
	require.NoError(t, err)
	require.Equal(t, codeCids1, codeCids2)

	// Check that the miner actor was migrated.
	minerActor, err := clientApi.StateGetActor(ctx, testMiner.ActorAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, codeCids1[manifest.MinerKey], minerActor.Code)

	// check EAM
	eamActor, err := newStateTree.GetActor(builtin.EthereumAddressManagerActorAddr)
	require.NoError(t, err)
	eamCodeCid, ok := codeCids1[manifest.EamKey]
	require.True(t, ok)
	require.Equal(t, eamCodeCid, eamActor.Code)

	// check the EthZeroAddress
	ethZeroAddr, err := (ethtypes.EthAddress{}).ToFilecoinAddress()
	require.NoError(t, err)
	ethZeroAddrID, err := newStateTree.LookupID(ethZeroAddr)
	require.NoError(t, err)
	ethZeroActor, err := newStateTree.GetActor(ethZeroAddrID)
	require.NoError(t, err)
	require.True(t, builtin2.IsEthAccountActor(ethZeroActor.Code))
	require.Equal(t, vm.EmptyObjectCid, ethZeroActor.Head)
}
