// stm: #integration
package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/itests/kit"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
)

func TestTapeFix(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001
	kit.QuietMiningLogs()

	var blocktime = 2 * time.Millisecond

	// The "before" case is disabled, because we need the builder to mock 32 GiB sectors to accurately repro this case
	// TODO: Make the mock sector size configurable and reenable this
	// t.Run("before", func(t *testing.T) { testTapeFix(t, b, blocktime, false) })
	t.Run("after", func(t *testing.T) { testTapeFix(t, blocktime, true) })
}

func testTapeFix(t *testing.T, blocktime time.Duration, after bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	networkVersion := network.Version4
	if after {
		networkVersion = network.Version5
	}

	_, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.GenesisNetworkVersion(networkVersion))
	ens.InterconnectAll().BeginMining(blocktime)

	sid, err := miner.PledgeSector(ctx)
	require.NoError(t, err)

	t.Log("All sectors is fsm")

	// If before, we expect the precommit to fail
	successState := api.SectorState(sealing.CommitFailed)
	failureState := api.SectorState(sealing.Proving)
	if after {
		// otherwise, it should succeed.
		successState, failureState = failureState, successState
	}

	for {
		st, err := miner.SectorsStatus(ctx, sid.Number, false)
		require.NoError(t, err)
		if st.State == successState {
			break
		}
		require.NotEqual(t, failureState, st.State)
		build.Clock.Sleep(100 * time.Millisecond)
		t.Log("WaitSeal")
	}
}
