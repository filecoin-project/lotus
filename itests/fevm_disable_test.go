package itests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestDisableFEVM(t *testing.T) {
	kit.QuietMiningLogs()

	nv19epoch := abi.ChainEpoch(100)
	testClient, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(),
		kit.UpgradeSchedule(stmgr.Upgrade{
			Network: network.Version18,
			Height:  -1,
		}, stmgr.Upgrade{
			Network:   network.Version19,
			Height:    nv19epoch,
			Migration: filcns.UpgradeActorsV11,
		},
		))

	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testClient.WaitTillChain(ctx, kit.HeightAtLeast(nv19epoch+5))

	_, err := testClient.EVM().DeployContractFromFilename(ctx, "contracts/Constructor.hex")

	fmt.Println(err)
	//bs := blockstore.NewAPIBlockstore(testClient)
	//ctxStore := gstStore.WrapBlockStore(ctx, bs)
	//
	//currTs, err := clientApi.ChainHead(ctx)
	//require.NoError(t, err)
	//
	//newStateTree, err := state.LoadStateTree(ctxStore, currTs.Blocks()[0].ParentStateRoot)
	//require.NoError(t, err)
	//
	//require.Equal(t, types.StateTreeVersion5, newStateTree.Version())
	//
	//codeIDsv11, ok := actors.GetActorCodeIDsFromManifest(actorstypes.Version11)
	//require.True(t, ok)
	//
	//// check the EAM actor
	//EAMActor, err := newStateTree.GetActor(builtin.EthereumAddressManagerActorAddr)
	//require.NoError(t, err)
	//EAMCodeID, ok := codeIDsv11[manifest.EamKey]
	//require.True(t, ok)
	//require.Equal(t, EAMCodeID, EAMActor.Code)
	//
	//// check the EthZeroAddress
	//ethZeroAddr, err := (ethtypes.EthAddress{}).ToFilecoinAddress()
	//require.NoError(t, err)
	//ethZeroAddrID, err := newStateTree.LookupID(ethZeroAddr)
	//require.NoError(t, err)
	//ethZeroActor, err := newStateTree.GetActor(ethZeroAddrID)
	//require.NoError(t, err)
	//require.True(t, builtin2.IsEthAccountActor(ethZeroActor.Code))
	//require.Equal(t, vm.EmptyObjectCid, ethZeroActor.Head)
}
