// stm: #integration
package itests

import (
	"bytes"
	"context"
	"testing"
	"time"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	inittypes "github.com/filecoin-project/go-state-types/builtin/v8/init"
	multisigtypes "github.com/filecoin-project/go-state-types/builtin/v8/multisig"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	lmultisig "github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/itests/multisig"
)

// TestMultisig does a basic test to exercise the multisig CLI commands
func TestMultisig(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001
	kit.QuietMiningLogs()

	blockTime := 5 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	multisig.RunMultisigTests(t, client)
}

// TestMultisigReentrant sends an infinitely recursive message to a multisig.
func TestMultisigReentrant(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001
	kit.QuietMiningLogs()

	ctx := context.Background()

	blockTime := 5 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)
	store := adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(client)))

	signer := client.DefaultKey.Address

	// Create the multisig
	cp, err := client.MsigCreate(ctx, 1, []address.Address{signer}, 0, big.Zero(), signer, big.Zero())
	require.NoError(t, err, "failed to create multisig (MsigCreate)")

	cm, err := client.MpoolPushMessage(ctx, &cp.Message, nil)
	require.NoError(t, err, "failed to create multisig (MpooPushMessage)")

	ml, err := client.StateWaitMsg(ctx, cm.Cid(), 5, 100, false)
	require.NoError(t, err, "failed to create multisig (StateWaitMsg)")
	require.Equal(t, ml.Receipt.ExitCode, exitcode.Ok)

	var execreturn inittypes.ExecReturn
	err = execreturn.UnmarshalCBOR(bytes.NewReader(ml.Receipt.Return))
	require.NoError(t, err, "failed to decode multisig create return")

	multisigAddress := execreturn.IDAddress

	// Add the multisig itself as a signer, do NOT increase the threshold
	ap, err := client.MsigAddPropose(ctx, multisigAddress, signer, multisigAddress, false)
	require.NoError(t, err, "failed to add multisig as signer (MsigAddPropose)")

	am, err := client.MpoolPushMessage(ctx, &ap.Message, nil)
	require.NoError(t, err, "failed to add multisig as signer (MpooPushMessage)")

	al, err := client.StateWaitMsg(ctx, am.Cid(), 5, 100, false)
	require.NoError(t, err, "failed to add multisig as signer (StateWaitMsg)")
	require.Equal(t, al.Receipt.ExitCode, exitcode.Ok)

	var propReturn multisigtypes.ProposeReturn
	err = propReturn.UnmarshalCBOR(bytes.NewReader(al.Receipt.Return))
	require.NoError(t, err, "failed to decode multisig propose return")
	require.True(t, propReturn.Applied, "expected proposal to apply the message")

	head, err := client.ChainHead(ctx)
	require.NoError(t, err)

	multisigActor, err := client.StateGetActor(ctx, multisigAddress, head.Key())
	require.NoError(t, err)

	mstate, err := lmultisig.Load(store, multisigActor)
	require.NoError(t, err)

	signers, err := mstate.Signers()
	require.NoError(t, err)

	require.Equal(t, 2, len(signers))
	require.Equal(t, multisigAddress, signers[1])

	// Send the reentrant tx -- it will try to approve itself (expects to be txid 1)

	approveParams, err := actors.SerializeParams(&multisigtypes.TxnIDParams{ID: 1})
	require.NoError(t, err)

	pp, err := client.MsigPropose(ctx, multisigAddress, multisigAddress, big.Zero(), signer, uint64(builtin.MethodsMultisig.Approve), approveParams)
	require.NoError(t, err)

	pm, err := client.MpoolPushMessage(ctx, &pp.Message, nil)
	require.NoError(t, err, "failed to send reentrant propose message (MpooPushMessage)")

	pl, err := client.StateWaitMsg(ctx, pm.Cid(), 5, 100, false)
	require.NoError(t, err, "failed to send reentrant propose message (StateWaitMsg)")
	require.Equal(t, pl.Receipt.ExitCode, exitcode.Ok)

	err = propReturn.UnmarshalCBOR(bytes.NewReader(pl.Receipt.Return))
	require.NoError(t, err, "failed to decode multisig propose return")
	require.True(t, propReturn.Applied, "expected proposal to apply the message")
	require.Equal(t, exitcode.Ok, propReturn.Code)

	sl, err := client.StateReplay(ctx, types.EmptyTSK, pm.Cid())
	require.NoError(t, err, "failed to replay reentrant propose message (StateWaitMsg)")

	require.Equal(t, 1024, countDepth(sl.ExecutionTrace), "failed: %s", sl.Error)
}

func countDepth(trace types.ExecutionTrace) int {
	if len(trace.Subcalls) == 0 {
		return 0
	}
	return countDepth(trace.Subcalls[0]) + 1
}
