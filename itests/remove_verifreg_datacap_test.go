package itests

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/build"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	migration "github.com/filecoin-project/go-state-types/builtin/v9/migration/test"
	miner9 "github.com/filecoin-project/go-state-types/builtin/v9/miner"
	verifregst "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/impl"
)

func TestNoRemoveDatacapFromVerifreg(t *testing.T) {
	ctx := context.Background()

	kit.QuietMiningLogs()

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

	testClient, testMiner, ens := kit.EnsembleMinimal(t, kit.MockProofs(),
		kit.RootVerifier(rootKey, abi.NewTokenAmount(bal.Int64())),
		kit.Account(verifier1Key, abi.NewTokenAmount(bal.Int64())),
		kit.Account(verifier2Key, abi.NewTokenAmount(bal.Int64())),
		kit.Account(verifiedClientKey, abi.NewTokenAmount(bal.Int64())))

	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	clientApi := testClient.FullNode.(*impl.FullNodeAPI)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Before the upgrade, we need to:
	// Setup a verified client
	// Publish (but NOT activate) a verified storage deal from that clien

	// get VRH
	//stm: @CHAIN_STATE_VERIFIED_REGISTRY_ROOT_KEY_001
	vrh, err := clientApi.StateVerifiedRegistryRootKey(ctx, types.TipSetKey{})
	fmt.Println(vrh.String())
	require.NoError(t, err)

	// import the root key.
	rootAddr, err := clientApi.WalletImport(ctx, &rootKey.KeyInfo)
	require.NoError(t, err)

	// import the verifier's key.
	verifier1Addr, err := clientApi.WalletImport(ctx, &verifier1Key.KeyInfo)
	require.NoError(t, err)

	verifier1IDAddr, err := clientApi.StateLookupID(ctx, verifier1Addr, types.EmptyTSK)
	require.NoError(t, err)

	verifier2Addr, err := clientApi.WalletImport(ctx, &verifier2Key.KeyInfo)
	require.NoError(t, err)

	verifier2IDAddr, err := clientApi.StateLookupID(ctx, verifier2Addr, types.EmptyTSK)
	require.NoError(t, err)

	// import the verified client's key.
	verifiedClientAddr, err := clientApi.WalletImport(ctx, &verifiedClientKey.KeyInfo)
	require.NoError(t, err)

	verifiedClientIDAddr, err := clientApi.StateLookupID(ctx, verifiedClientAddr, types.EmptyTSK)
	require.NoError(t, err)

	params, err := actors.SerializeParams(&verifregst.AddVerifierParams{Address: verifier1Addr, Allowance: big.NewInt(100000000000)})
	require.NoError(t, err)

	msg := &types.Message{
		From:   rootAddr,
		To:     verifreg.Address,
		Method: verifreg.Methods.AddVerifier,
		Params: params,
		Value:  big.Zero(),
	}

	sm, err := clientApi.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err, "AddVerifier failed")

	//stm: @CHAIN_STATE_WAIT_MSG_001
	res, err := clientApi.StateWaitMsg(ctx, sm.Cid(), 1, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.True(t, res.Receipt.ExitCode.IsSuccess())

	params, err = actors.SerializeParams(&verifregst.AddVerifierParams{Address: verifier2Addr, Allowance: big.NewInt(100000000000)})
	require.NoError(t, err)

	msg = &types.Message{
		From:   rootAddr,
		To:     verifreg.Address,
		Method: verifreg.Methods.AddVerifier,
		Params: params,
		Value:  big.Zero(),
	}

	sm, err = clientApi.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err, "AddVerifier failed")

	//stm: @CHAIN_STATE_WAIT_MSG_001
	res, err = clientApi.StateWaitMsg(ctx, sm.Cid(), 1, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.True(t, res.Receipt.ExitCode.IsSuccess())

	// assign datacap to a client
	datacapToAssign := big.NewInt(10000)

	params, err = actors.SerializeParams(&verifregst.AddVerifiedClientParams{Address: verifiedClientAddr, Allowance: datacapToAssign})
	require.NoError(t, err)

	msg = &types.Message{
		From:   verifier1Addr,
		To:     verifreg.Address,
		Method: verifreg.Methods.AddVerifiedClient,
		Params: params,
		Value:  big.Zero(),
	}

	sm, err = clientApi.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)

	//stm: @CHAIN_STATE_WAIT_MSG_001
	res, err = clientApi.StateWaitMsg(ctx, sm.Cid(), 1, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.True(t, res.Receipt.ExitCode.IsSuccess())

	// check datacap balance
	//stm: @CHAIN_STATE_VERIFIED_CLIENT_STATUS_001
	dc, err := clientApi.StateVerifiedClientStatus(ctx, verifiedClientAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, *dc, datacapToAssign)

	label, err := markettypes.NewLabelFromString("")
	require.NoError(t, err)

	dealStartEpoch := abi.ChainEpoch(1000)
	dealProposal := markettypes.DealProposal{
		PieceCID:             migration.MakeCID("1", &markettypes.PieceCIDPrefix),
		PieceSize:            1024,
		Client:               verifiedClientAddr,
		Provider:             testMiner.ActorAddr,
		Label:                label,
		StartEpoch:           dealStartEpoch,
		EndEpoch:             abi.ChainEpoch(1000_000),
		StoragePricePerEpoch: big.Zero(),
		ProviderCollateral:   big.Zero(),
		ClientCollateral:     big.Zero(),
		VerifiedDeal:         true,
	}

	serializedProposal := new(bytes.Buffer)
	err = dealProposal.MarshalCBOR(serializedProposal)
	require.NoError(t, err)

	sig, err := clientApi.WalletSign(ctx, verifiedClientAddr, serializedProposal.Bytes())

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

	m, err := clientApi.MpoolPushMessage(ctx, &types.Message{
		To:     builtin.StorageMarketActorAddr,
		From:   testMiner.OwnerKey.Address,
		Value:  types.FromFil(0),
		Method: builtin.MethodsMarket.PublishStorageDeals,
		Params: serializedParams.Bytes(),
	}, nil)
	require.NoError(t, err)

	r, err := clientApi.StateWaitMsg(ctx, m.Cid(), 2, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.True(t, r.Receipt.ExitCode.IsSuccess())

	ret, err := market.DecodePublishStorageDealsReturn(r.Receipt.Return, build.TestNetworkVersion)
	require.NoError(t, err)
	valid, _, err := ret.IsDealValid(0)
	require.NoError(t, err)
	require.True(t, valid)
	dealIds, err := ret.DealIDs()
	require.NoError(t, err)

	// END OF VERIFIED CLIENT BOILERPLATE

	verifiedClientDcap, err := clientApi.StateVerifiedClientStatus(ctx, verifiedClientIDAddr, types.EmptyTSK)
	require.NoError(t, err)

	// The client has already spent datacap equal to the deal's size -- this will be found in the VerifiedRegistryActor
	require.Equal(t, big.Sub(datacapToAssign, big.NewIntUnsigned(uint64(dealProposal.PieceSize))), *verifiedClientDcap)

	verifregDcapBefore, err := clientApi.StateVerifiedClientStatus(ctx, builtin.VerifiedRegistryActorAddr, types.EmptyTSK)
	require.NoError(t, err)

	// Verifreg should now have the datacap for this deal
	require.Equal(t, big.NewIntUnsigned(uint64(dealProposal.PieceSize)), *verifregDcapBefore)

	// Now let's try to remove datacap from Verifreg
	removeProposal := verifregst.RemoveDataCapProposal{
		VerifiedClient:    verifreg.Address,
		DataCapAmount:     *verifregDcapBefore,
		RemovalProposalID: verifregst.RmDcProposalID{ProposalID: 0},
	}

	buf := bytes.Buffer{}
	buf.WriteString(verifregst.SignatureDomainSeparation_RemoveDataCap)
	require.NoError(t, removeProposal.MarshalCBOR(&buf), "failed to marshal proposal")

	removeProposalSer := buf.Bytes()

	verifier1Sig, err := clientApi.WalletSign(ctx, verifier1Addr, removeProposalSer)
	require.NoError(t, err, "failed to sign proposal")

	removeRequest1 := verifregst.RemoveDataCapRequest{
		Verifier:          verifier1IDAddr,
		VerifierSignature: *verifier1Sig,
	}

	verifier2Sig, err := clientApi.WalletSign(ctx, verifier2Addr, removeProposalSer)
	require.NoError(t, err, "failed to sign proposal")

	removeRequest2 := verifregst.RemoveDataCapRequest{
		Verifier:          verifier2IDAddr,
		VerifierSignature: *verifier2Sig,
	}

	removeDataCapParams := verifregst.RemoveDataCapParams{
		VerifiedClientToRemove: verifreg.Address,
		DataCapAmountToRemove:  *verifregDcapBefore,
		VerifierRequest1:       removeRequest1,
		VerifierRequest2:       removeRequest2,
	}

	params, aerr := actors.SerializeParams(&removeDataCapParams)
	require.NoError(t, aerr)

	m, err = clientApi.MpoolPushMessage(ctx, &types.Message{
		From:   rootAddr,
		To:     verifreg.Address,
		Method: verifreg.Methods.RemoveVerifiedClientDataCap,
		Params: params,
		Value:  big.Zero(),
	}, nil)
	require.NoError(t, err) // This should actually fail

	r, err = clientApi.StateWaitMsg(ctx, m.Cid(), 2, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.True(t, r.Receipt.ExitCode.IsSuccess())

	verifregDatacapAfter, err := clientApi.StateVerifiedClientStatus(ctx, builtin.VerifiedRegistryActorAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Nil(t, verifregDatacapAfter) // Verifreg datacap will be removed from datacap actor

	minerInfo, err := testClient.StateMinerInfo(ctx, testMiner.ActorAddr, types.EmptyTSK)
	require.NoError(t, err)

	spt, err := miner.SealProofTypeFromSectorSize(minerInfo.SectorSize, build.TestNetworkVersion)
	require.NoError(t, err)

	preCommitParams := miner9.PreCommitSectorParams{
		SealProof:     spt,
		SectorNumber:  1000,
		SealedCID:     migration.MakeCID("sector", &miner9.SealedCIDPrefix),
		SealRandEpoch: dealStartEpoch - 5,
		DealIDs:       dealIds,
		Expiration:    dealProposal.EndEpoch,
	}

	serializedParams = new(bytes.Buffer)
	require.NoError(t, preCommitParams.MarshalCBOR(serializedParams))

	m, err = clientApi.MpoolPushMessage(ctx, &types.Message{
		To:     testMiner.ActorAddr,
		From:   testMiner.OwnerKey.Address,
		Value:  types.FromFil(0),
		Method: builtin.MethodsMiner.PreCommitSector,
		Params: serializedParams.Bytes(),
	}, nil)
	require.NoError(t, err)

	r, err = clientApi.StateWaitMsg(ctx, m.Cid(), 2, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.True(t, r.Receipt.ExitCode.IsSuccess())

	testClient.WaitTillChain(ctx, kit.HeightAtLeast(r.Height+miner9.PreCommitChallengeDelay+5))

	proveCommitParams := miner9.ProveCommitSectorParams{
		SectorNumber: preCommitParams.SectorNumber,
		Proof:        []byte{0xde, 0xad, 0xbe, 0xef},
	}

	serializedParams = new(bytes.Buffer)
	require.NoError(t, proveCommitParams.MarshalCBOR(serializedParams))

	m, err = clientApi.MpoolPushMessage(ctx, &types.Message{
		To:     testMiner.ActorAddr,
		From:   testMiner.OwnerKey.Address,
		Value:  types.FromFil(0),
		Method: builtin.MethodsMiner.ProveCommitSector,
		Params: serializedParams.Bytes(),
	}, nil)
	require.NoError(t, err)

	r, err = clientApi.StateWaitMsg(ctx, m.Cid(), 2, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.True(t, r.Receipt.ExitCode.IsSuccess())
}
