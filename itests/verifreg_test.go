// stm: #integration
package itests

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	verifregst "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/network"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/impl"
)

func TestVerifiedClientTopUp(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001
	blockTime := 100 * time.Millisecond

	test := func(nv network.Version, shouldWork bool) func(*testing.T) {
		return func(t *testing.T) {
			rootKey, err := key.GenerateKey(types.KTSecp256k1)
			require.NoError(t, err)

			verifierKey, err := key.GenerateKey(types.KTSecp256k1)
			require.NoError(t, err)

			verifiedClientKey, err := key.GenerateKey(types.KTBLS)
			require.NoError(t, err)

			bal, err := types.ParseFIL("100fil")
			require.NoError(t, err)

			node, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(),
				kit.RootVerifier(rootKey, abi.NewTokenAmount(bal.Int64())),
				kit.Account(verifierKey, abi.NewTokenAmount(bal.Int64())), // assign some balance to the verifier so they can send an AddClient message.
				kit.GenesisNetworkVersion(nv))

			ens.InterconnectAll().BeginMining(blockTime)

			api := node.FullNode.(*impl.FullNodeAPI)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// get VRH
			//stm: @CHAIN_STATE_VERIFIED_REGISTRY_ROOT_KEY_001
			vrh, err := api.StateVerifiedRegistryRootKey(ctx, types.TipSetKey{})
			fmt.Println(vrh.String())
			require.NoError(t, err)

			// import the root key.
			rootAddr, err := api.WalletImport(ctx, &rootKey.KeyInfo)
			require.NoError(t, err)

			// import the verifier's key.
			verifierAddr, err := api.WalletImport(ctx, &verifierKey.KeyInfo)
			require.NoError(t, err)

			// import the verified client's key.
			verifiedClientAddr, err := api.WalletImport(ctx, &verifiedClientKey.KeyInfo)
			require.NoError(t, err)

			params, err := actors.SerializeParams(&verifregst.AddVerifierParams{Address: verifierAddr, Allowance: big.NewInt(100000000000)})
			require.NoError(t, err)

			msg := &types.Message{
				From:   rootAddr,
				To:     verifreg.Address,
				Method: verifreg.Methods.AddVerifier,
				Params: params,
				Value:  big.Zero(),
			}

			sm, err := api.MpoolPushMessage(ctx, msg, nil)
			require.NoError(t, err, "AddVerifier failed")

			//stm: @CHAIN_STATE_WAIT_MSG_001
			res, err := api.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
			require.NoError(t, err)
			require.EqualValues(t, 0, res.Receipt.ExitCode)

			// assign datacap to a client
			datacap := big.NewInt(10000)

			params, err = actors.SerializeParams(&verifregst.AddVerifiedClientParams{Address: verifiedClientAddr, Allowance: datacap})
			require.NoError(t, err)

			msg = &types.Message{
				From:   verifierAddr,
				To:     verifreg.Address,
				Method: verifreg.Methods.AddVerifiedClient,
				Params: params,
				Value:  big.Zero(),
			}

			sm, err = api.MpoolPushMessage(ctx, msg, nil)
			require.NoError(t, err)

			//stm: @CHAIN_STATE_WAIT_MSG_001
			res, err = api.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
			require.NoError(t, err)
			require.EqualValues(t, 0, res.Receipt.ExitCode)

			// check datacap balance
			//stm: @CHAIN_STATE_VERIFIED_CLIENT_STATUS_001
			dcap, err := api.StateVerifiedClientStatus(ctx, verifiedClientAddr, types.EmptyTSK)
			require.NoError(t, err)
			require.Equal(t, *dcap, datacap)

			// try to assign datacap to the same client should fail for actor v4 and below
			params, err = actors.SerializeParams(&verifregst.AddVerifiedClientParams{Address: verifiedClientAddr, Allowance: datacap})
			if err != nil {
				t.Fatal(err)
			}

			msg = &types.Message{
				From:   verifierAddr,
				To:     verifreg.Address,
				Method: verifreg.Methods.AddVerifiedClient,
				Params: params,
				Value:  big.Zero(),
			}

			_, err = api.MpoolPushMessage(ctx, msg, nil)
			if shouldWork && err != nil {
				t.Fatal("expected nil err", err)
			}

			if !shouldWork && (err == nil || !strings.Contains(err.Error(), "verified client already exists")) {
				t.Fatal("Add datacap to an existing verified client should fail")
			}
		}
	}

	t.Run("nv12", test(network.Version12, false))
	t.Run("nv13", test(network.Version13, true))
}

func TestRemoveDataCap(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001
	blockTime := 100 * time.Millisecond

	rootKey, err := key.GenerateKey(types.KTSecp256k1)
	require.NoError(t, err)

	verifier1Key, err := key.GenerateKey(types.KTSecp256k1)
	require.NoError(t, err)

	verifier2Key, err := key.GenerateKey(types.KTSecp256k1)
	require.NoError(t, err)

	verifiedClientKey, err := key.GenerateKey(types.KTBLS)
	require.NoError(t, err)

	bal, err := types.ParseFIL("100fil")
	require.NoError(t, err)

	node, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(),
		kit.RootVerifier(rootKey, abi.NewTokenAmount(bal.Int64())),
		kit.Account(verifier1Key, abi.NewTokenAmount(bal.Int64())),
		kit.Account(verifier2Key, abi.NewTokenAmount(bal.Int64())),
		kit.Account(verifiedClientKey, abi.NewTokenAmount(bal.Int64())),
	)

	ens.InterconnectAll().BeginMining(blockTime)

	api := node.FullNode.(*impl.FullNodeAPI)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// get VRH
	//stm: @CHAIN_STATE_VERIFIED_REGISTRY_ROOT_KEY_001
	vrh, err := api.StateVerifiedRegistryRootKey(ctx, types.TipSetKey{})
	fmt.Println(vrh.String())
	require.NoError(t, err)

	// import the root key.
	rootAddr, err := api.WalletImport(ctx, &rootKey.KeyInfo)
	require.NoError(t, err)

	// import the verifiers' keys.
	verifier1Addr, err := api.WalletImport(ctx, &verifier1Key.KeyInfo)
	require.NoError(t, err)

	verifier2Addr, err := api.WalletImport(ctx, &verifier2Key.KeyInfo)
	require.NoError(t, err)

	// import the verified client's key.
	verifiedClientAddr, err := api.WalletImport(ctx, &verifiedClientKey.KeyInfo)
	require.NoError(t, err)

	// resolve all keys

	verifier1ID, err := api.StateLookupID(ctx, verifier1Addr, types.EmptyTSK)
	require.NoError(t, err)

	verifier2ID, err := api.StateLookupID(ctx, verifier2Addr, types.EmptyTSK)
	require.NoError(t, err)

	verifiedClientID, err := api.StateLookupID(ctx, verifiedClientAddr, types.EmptyTSK)
	require.NoError(t, err)

	// make the 2 verifiers

	makeVerifier := func(addr address.Address) error {
		allowance := big.NewInt(100000000000)
		params, aerr := actors.SerializeParams(&verifregst.AddVerifierParams{Address: addr, Allowance: allowance})
		require.NoError(t, aerr)

		msg := &types.Message{
			From:   rootAddr,
			To:     verifreg.Address,
			Method: verifreg.Methods.AddVerifier,
			Params: params,
			Value:  big.Zero(),
		}

		sm, err := api.MpoolPushMessage(ctx, msg, nil)
		require.NoError(t, err, "AddVerifier failed")

		//stm: @CHAIN_STATE_WAIT_MSG_001
		res, err := api.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
		require.NoError(t, err)
		require.EqualValues(t, 0, res.Receipt.ExitCode)

		verifierAllowance, err := api.StateVerifierStatus(ctx, addr, types.EmptyTSK)
		require.NoError(t, err)
		require.Equal(t, allowance, *verifierAllowance)

		return nil
	}

	require.NoError(t, makeVerifier(verifier1Addr))
	require.NoError(t, makeVerifier(verifier2Addr))

	// assign datacap to a client
	datacap := big.NewInt(10000)

	params, err := actors.SerializeParams(&verifregst.AddVerifiedClientParams{Address: verifiedClientAddr, Allowance: datacap})
	require.NoError(t, err)

	msg := &types.Message{
		From:   verifier1Addr,
		To:     verifreg.Address,
		Method: verifreg.Methods.AddVerifiedClient,
		Params: params,
		Value:  big.Zero(),
	}

	sm, err := api.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)

	//stm: @CHAIN_STATE_WAIT_MSG_001
	res, err := api.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	// check datacap balance
	//stm: @CHAIN_STATE_VERIFIED_CLIENT_STATUS_001
	dcap, err := api.StateVerifiedClientStatus(ctx, verifiedClientAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, *dcap, datacap)

	// helper to create removedatacap message
	makeRemoveDatacapMsg := func(removeDatacap big.Int, proposalID uint64) *types.Message {
		removeProposal := verifregst.RemoveDataCapProposal{
			VerifiedClient:    verifiedClientID,
			DataCapAmount:     removeDatacap,
			RemovalProposalID: verifregst.RmDcProposalID{ProposalID: proposalID},
		}

		buf := bytes.Buffer{}
		buf.WriteString(verifregst.SignatureDomainSeparation_RemoveDataCap)
		require.NoError(t, removeProposal.MarshalCBOR(&buf), "failed to marshal proposal")

		removeProposalSer := buf.Bytes()

		verifier1Sig, err := api.WalletSign(ctx, verifier1Addr, removeProposalSer)
		require.NoError(t, err, "failed to sign proposal")

		removeRequest1 := verifregst.RemoveDataCapRequest{
			Verifier:          verifier1ID,
			VerifierSignature: *verifier1Sig,
		}

		verifier2Sig, err := api.WalletSign(ctx, verifier2Addr, removeProposalSer)
		require.NoError(t, err, "failed to sign proposal")

		removeRequest2 := verifregst.RemoveDataCapRequest{
			Verifier:          verifier2ID,
			VerifierSignature: *verifier2Sig,
		}

		removeDataCapParams := verifregst.RemoveDataCapParams{
			VerifiedClientToRemove: verifiedClientAddr,
			DataCapAmountToRemove:  removeDatacap,
			VerifierRequest1:       removeRequest1,
			VerifierRequest2:       removeRequest2,
		}

		params, aerr := actors.SerializeParams(&removeDataCapParams)
		require.NoError(t, aerr)

		msg = &types.Message{
			From:   rootAddr,
			To:     verifreg.Address,
			Method: verifreg.Methods.RemoveVerifiedClientDataCap,
			Params: params,
			Value:  big.Zero(),
		}

		return msg
	}

	// let's take away half the client's datacap now

	removeDatacap := big.Div(datacap, big.NewInt(2))
	// proposal ids are 0 the first time
	removeMsg := makeRemoveDatacapMsg(removeDatacap, 0)

	sm, err = api.MpoolPushMessage(ctx, removeMsg, nil)
	require.NoError(t, err, "RemoveDataCap failed")

	//stm: @CHAIN_STATE_WAIT_MSG_001
	res, err = api.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	// check datacap balance
	//stm: @CHAIN_STATE_VERIFIED_CLIENT_STATUS_001
	dcap, err = api.StateVerifiedClientStatus(ctx, verifiedClientAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, *dcap, big.Sub(datacap, removeDatacap))

	// now take away the second half!

	// proposal ids are 1 the second time
	removeMsg = makeRemoveDatacapMsg(removeDatacap, 1)

	sm, err = api.MpoolPushMessage(ctx, removeMsg, nil)
	require.NoError(t, err, "RemoveDataCap failed")

	//stm: @CHAIN_STATE_WAIT_MSG_001
	res, err = api.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	// check datacap balance
	//stm: @CHAIN_STATE_VERIFIED_CLIENT_STATUS_001
	dcap, err = api.StateVerifiedClientStatus(ctx, verifiedClientAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Nil(t, dcap, "expected datacap to be nil")
}
