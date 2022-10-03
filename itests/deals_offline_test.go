// stm: #integration
package itests

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	commcid "github.com/filecoin-project/go-fil-commcid"
	commp "github.com/filecoin-project/go-fil-commp-hashhash"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	verifregtypes "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/impl"
)

func TestOfflineDealFlow(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001
	//stm: @CLIENT_DATA_CALCULATE_COMMP_001, @CLIENT_DATA_GENERATE_CAR_001, @CLIENT_DATA_GET_DEAL_PIECE_CID_001, @CLIENT_DATA_GET_DEAL_PIECE_CID_001
	runTest := func(t *testing.T, fastRet bool, upscale abi.PaddedPieceSize) {
		ctx := context.Background()
		client, miner, ens := kit.EnsembleMinimal(t, kit.WithAllSubsystems()) // no mock proofs
		ens.InterconnectAll().BeginMining(250 * time.Millisecond)

		dh := kit.NewDealHarness(t, client, miner, miner)

		// Create a random file and import on the client.
		res, inFile := client.CreateImportFile(ctx, 1, 200)

		// Get the piece size and commP
		rootCid := res.Root
		pieceInfo, err := client.ClientDealPieceCID(ctx, rootCid)
		require.NoError(t, err)
		t.Log("FILE CID:", rootCid)

		// test whether padding works as intended
		if upscale > 0 {
			newRawCp, err := commp.PadCommP(
				pieceInfo.PieceCID.Hash()[len(pieceInfo.PieceCID.Hash())-32:],
				uint64(pieceInfo.PieceSize),
				uint64(upscale),
			)
			require.NoError(t, err)

			pieceInfo.PieceSize = upscale
			pieceInfo.PieceCID, err = commcid.DataCommitmentV1ToCID(newRawCp)
			require.NoError(t, err)
		}

		dp := dh.DefaultStartDealParams()
		dp.DealStartEpoch = abi.ChainEpoch(4 << 10)
		dp.FastRetrieval = fastRet
		// Replace with params for manual storage deal (offline deal)
		dp.Data = &storagemarket.DataRef{
			TransferType: storagemarket.TTManual,
			Root:         rootCid,
			PieceCid:     &pieceInfo.PieceCID,
			PieceSize:    pieceInfo.PieceSize.Unpadded(),
		}

		proposalCid := dh.StartDeal(ctx, dp)

		//stm: @CLIENT_STORAGE_DEALS_GET_001
		// Wait for the deal to reach StorageDealCheckForAcceptance on the client
		cd, err := client.ClientGetDealInfo(ctx, *proposalCid)
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			cd, _ := client.ClientGetDealInfo(ctx, *proposalCid)
			return cd.State == storagemarket.StorageDealCheckForAcceptance
		}, 30*time.Second, 1*time.Second, "actual deal status is %s", storagemarket.DealStates[cd.State])

		// Create a CAR file from the raw file
		carFileDir := t.TempDir()
		carFilePath := filepath.Join(carFileDir, "out.car")
		err = client.ClientGenCar(ctx, lapi.FileRef{Path: inFile}, carFilePath)
		require.NoError(t, err)

		// Import the CAR file on the miner - this is the equivalent to
		// transferring the file across the wire in a normal (non-offline) deal
		err = miner.DealsImportData(ctx, *proposalCid, carFilePath)
		require.NoError(t, err)

		// Wait for the deal to be published
		dh.WaitDealPublished(ctx, proposalCid)

		t.Logf("deal published, retrieving")

		// Retrieve the deal
		outFile := dh.PerformRetrieval(ctx, proposalCid, rootCid, false)

		kit.AssertFilesEqual(t, inFile, outFile)

	}

	t.Run("stdretrieval", func(t *testing.T) { runTest(t, false, 0) })
	t.Run("fastretrieval", func(t *testing.T) { runTest(t, true, 0) })
	t.Run("fastretrieval", func(t *testing.T) { runTest(t, true, 1024) })
}

func TestGetAllocationFromDealId(t *testing.T) {
	ctx := context.Background()

	rootKey, err := key.GenerateKey(types.KTSecp256k1)
	require.NoError(t, err)

	verifierKey, err := key.GenerateKey(types.KTSecp256k1)
	require.NoError(t, err)

	verifiedClientKey, err := key.GenerateKey(types.KTBLS)
	require.NoError(t, err)

	bal, err := types.ParseFIL("100fil")
	require.NoError(t, err)

	node, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(),
		kit.RootVerifier(rootKey, abi.NewTokenAmount(bal.Int64())),
		kit.Account(verifierKey, abi.NewTokenAmount(bal.Int64())), // assign some balance to the verifier so they can send an AddClient message.
		kit.Account(verifiedClientKey, abi.NewTokenAmount(bal.Int64())),
	)

	ens.InterconnectAll().BeginMining(250 * time.Millisecond)

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

	params, err := actors.SerializeParams(&verifregtypes.AddVerifierParams{Address: verifierAddr, Allowance: big.NewInt(100000000000)})
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

	params, err = actors.SerializeParams(&verifregtypes.AddVerifiedClientParams{Address: verifiedClientAddr, Allowance: datacap})
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

	if !dcap.Equals(datacap) {
		t.Fatal("")
	}

	// Create a random file and import on the client.
	importedFile, _ := node.CreateImportFile(ctx, 1, 200)

	// Get the piece size and commP
	rootCid := importedFile.Root
	pieceInfo, err := node.ClientDealPieceCID(ctx, rootCid)
	require.NoError(t, err)
	t.Log("FILE CID:", rootCid)

	label, err := markettypes.NewLabelFromString("")
	require.NoError(t, err)

	dealProposal := markettypes.DealProposal{
		PieceCID:             pieceInfo.PieceCID,
		PieceSize:            pieceInfo.PieceSize,
		Client:               verifiedClientAddr,
		Provider:             miner.ActorAddr,
		Label:                label,
		StartEpoch:           abi.ChainEpoch(1000_000),
		EndEpoch:             abi.ChainEpoch(2000_000),
		StoragePricePerEpoch: big.Zero(),
		ProviderCollateral:   big.Zero(),
		ClientCollateral:     big.Zero(),
		VerifiedDeal:         true,
	}

	serializedProposal := new(bytes.Buffer)
	err = dealProposal.MarshalCBOR(serializedProposal)
	require.NoError(t, err)

	sig, err := api.WalletSign(ctx, verifiedClientAddr, serializedProposal.Bytes())

	publishDealParams := markettypes.PublishStorageDealsParams{
		Deals: []markettypes.ClientDealProposal{{
			Proposal: dealProposal,
			ClientSignature: crypto.Signature{
				Type: crypto.SigTypeBLS,
				Data: sig.Data,
			},
		}},
	}

	serializedParams := new(bytes.Buffer)
	require.NoError(t, publishDealParams.MarshalCBOR(serializedParams))

	m, err := node.MpoolPushMessage(ctx, &types.Message{
		To:     builtin.StorageMarketActorAddr,
		From:   miner.OwnerKey.Address,
		Value:  types.FromFil(0),
		Method: builtin.MethodsMarket.PublishStorageDeals,
		Params: serializedParams.Bytes(),
	}, nil)
	require.NoError(t, err)

	r, err := node.StateWaitMsg(ctx, m.Cid(), 2, lapi.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, r.Receipt.ExitCode)

	ret, err := market.DecodePublishStorageDealsReturn(r.Receipt.Return, build.TestNetworkVersion)
	require.NoError(t, err)
	valid, _, err := ret.IsDealValid(0)
	require.NoError(t, err)
	require.True(t, valid)
	dealIds, err := ret.DealIDs()
	require.NoError(t, err)

	dealInfo, err := api.StateMarketStorageDeal(ctx, dealIds[0], types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, verifregtypes.AllocationId(0), dealInfo.State.VerifiedClaim) // Allocation in State should not be set yet, because it's in the allocation map

	allocation, err := api.StateGetDealAllocation(ctx, dealIds[0], types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, dealProposal.PieceCID, allocation.Data)

	// Just another way of getting the allocation if we don't have the deal ID
	allocation, err = api.StateGetAllocation(ctx, verifiedClientAddr, 3, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, dealProposal.PieceCID, allocation.Data)
}
