package itests

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	miner11 "github.com/filecoin-project/go-state-types/builtin/v11/miner"
	power11 "github.com/filecoin-project/go-state-types/builtin/v11/power"
	adt11 "github.com/filecoin-project/go-state-types/builtin/v11/util/adt"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	migration "github.com/filecoin-project/go-state-types/builtin/v9/migration/test"
	miner9 "github.com/filecoin-project/go-state-types/builtin/v9/miner"
	verifregst "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/manifest"
	"github.com/filecoin-project/go-state-types/network"
	gstStore "github.com/filecoin-project/go-state-types/store"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	builtin2 "github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/datacap"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/system"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/impl"
)

func TestMigrationNV17(t *testing.T) {
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

	nv17epoch := abi.ChainEpoch(1000)
	testClient, testMiner, ens := kit.EnsembleMinimal(t, kit.MockProofs(),
		kit.RootVerifier(rootKey, abi.NewTokenAmount(bal.Int64())),
		kit.Account(verifier1Key, abi.NewTokenAmount(bal.Int64())),
		kit.Account(verifier2Key, abi.NewTokenAmount(bal.Int64())),
		kit.Account(verifiedClientKey, abi.NewTokenAmount(bal.Int64())),
		kit.UpgradeSchedule(stmgr.Upgrade{
			Network: network.Version16,
			Height:  -1,
		}, stmgr.Upgrade{
			Network:   network.Version17,
			Height:    nv17epoch,
			Migration: filcns.UpgradeActorsV9,
		},
		))

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

	dealProposal := markettypes.DealProposal{
		PieceCID:             migration.MakeCID("1", &markettypes.PieceCIDPrefix),
		PieceSize:            1024,
		Client:               verifiedClientAddr,
		Provider:             testMiner.ActorAddr,
		Label:                label,
		StartEpoch:           nv17epoch + 500,
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
	require.NoError(t, err)

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

	ret, err := market.DecodePublishStorageDealsReturn(r.Receipt.Return, network.Version16)
	require.NoError(t, err)
	valid, _, err := ret.IsDealValid(0)
	require.NoError(t, err)
	require.True(t, valid)
	dealIds, err := ret.DealIDs()
	require.NoError(t, err)

	currNV, err := clientApi.StateNetworkVersion(ctx, types.EmptyTSK)
	require.NoError(t, err)
	if currNV >= network.Version17 {
		// if we moved too slowly and are already at v17, abort the test here with "success"
		// it's not actually gonna test what we want, but the alternative is flakiness...
		fmt.Println("early termination -- test reached the migration too quickly!")
		return
	}

	testClient.WaitTillChain(ctx, kit.HeightAtLeast(nv17epoch+5))

	// Now that we have upgraded, we need to:
	// - confirm that the pending deal state is correct (allocation, entry in market pending allocations, etc.)
	// - activate the deal, confirm it succeeds, and has a corresponding claim

	bs := blockstore.NewAPIBlockstore(testClient)
	ctxStore := gstStore.WrapBlockStore(ctx, bs)

	currTs, err := clientApi.ChainHead(ctx)
	require.NoError(t, err)

	newStateTree, err := state.LoadStateTree(ctxStore, currTs.Blocks()[0].ParentStateRoot)
	require.NoError(t, err)

	datacapAct, err := newStateTree.GetActor(builtin.DatacapActorAddr)
	require.NoError(t, err)

	datacapSt, err := datacap.Load(ctxStore, datacapAct)
	require.NoError(t, err)

	ok, dcap, err := datacapSt.VerifiedClientDataCap(verifiedClientIDAddr)
	require.NoError(t, err)

	require.True(t, ok)
	// The client has already spent datacap equal to the deal's size -- this will be found in the VerifiedRegistryActor
	require.Equal(t, big.Sub(datacapToAssign, big.NewIntUnsigned(uint64(dealProposal.PieceSize))), dcap)

	ok, dcap, err = datacapSt.VerifiedClientDataCap(builtin.VerifiedRegistryActorAddr)
	require.NoError(t, err)

	require.True(t, ok)
	require.Equal(t, big.NewIntUnsigned(uint64(dealProposal.PieceSize)), dcap)

	// The deal has a pending allocation
	marketAct, err := newStateTree.GetActor(builtin.StorageMarketActorAddr)
	require.NoError(t, err)

	marketSt, err := market.Load(ctxStore, marketAct)
	require.NoError(t, err)

	allocationId, err := marketSt.GetAllocationIdForPendingDeal(dealIds[0])
	require.NoError(t, err)
	require.Equal(t, verifregst.AllocationId(1), allocationId)

	minerInfo, err := testClient.StateMinerInfo(ctx, testMiner.ActorAddr, types.EmptyTSK)
	require.NoError(t, err)

	spt, err := miner.SealProofTypeFromSectorSize(minerInfo.SectorSize, network.Version17, false)
	require.NoError(t, err)

	preCommitParams := miner9.PreCommitSectorParams{
		SealProof:     spt,
		SectorNumber:  1000,
		SealedCID:     migration.MakeCID("sector", &miner9.SealedCIDPrefix),
		SealRandEpoch: nv17epoch,
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

	// Yay, the deal has been activated! Let's assert that it has a claim.

	currTs, err = clientApi.ChainHead(ctx)
	require.NoError(t, err)

	newStateTree, err = state.LoadStateTree(ctxStore, currTs.Blocks()[0].ParentStateRoot)
	require.NoError(t, err)

	verifregAct, err := newStateTree.GetActor(builtin.VerifiedRegistryActorAddr)
	require.NoError(t, err)

	verifregSt, err := verifreg.Load(ctxStore, verifregAct)
	require.NoError(t, err)

	claims, err := verifregSt.GetClaims(testMiner.ActorAddr)
	require.NoError(t, err)

	require.Equal(t, len(claims), 1)
	claim, ok := claims[1]
	require.True(t, ok)

	claimerIdAddr, err := address.NewIDAddress(uint64(claim.Client))
	require.NoError(t, err)

	require.Equal(t, verifiedClientIDAddr, claimerIdAddr)

	// And that the deal no longer has a pending allocation

	marketAct, err = newStateTree.GetActor(builtin.StorageMarketActorAddr)
	require.NoError(t, err)

	marketSt, err = market.Load(ctxStore, marketAct)
	require.NoError(t, err)

	allocationId, err = marketSt.GetAllocationIdForPendingDeal(dealIds[0])
	require.NoError(t, err)
	require.Equal(t, verifregst.NoAllocationID, allocationId)

	testClient.WaitTillChain(ctx, kit.HeightAtLeast(dealProposal.StartEpoch+5))

	currTs, err = clientApi.ChainHead(ctx)
	require.NoError(t, err)

	cso, err := clientApi.StateCompute(ctx, currTs.Height(), nil, currTs.Key())
	require.NoError(t, err)

	// cso.Trace[len(cso.Trace) - 1] because Cron is always the last entry in an InvocResult
	// Subcalls [1] because Cron calls Power before Market
	marketCronTrace := cso.Trace[len(cso.Trace)-1].ExecutionTrace.Subcalls[1]
	require.Equal(t, builtin.CronActorAddr, marketCronTrace.Msg.From)
	require.Equal(t, builtin.StorageMarketActorAddr, marketCronTrace.Msg.To)
	require.True(t, marketCronTrace.MsgRct.ExitCode.IsSuccess())

	// Now let's make sure the verified client can still use its balance for new deals

	dealProposal.PieceCID = migration.MakeCID("2", &markettypes.PieceCIDPrefix)
	dealProposal.StartEpoch = currTs.Height() + 1000

	serializedProposal = new(bytes.Buffer)
	err = dealProposal.MarshalCBOR(serializedProposal)
	require.NoError(t, err)

	sig, err = clientApi.WalletSign(ctx, verifiedClientAddr, serializedProposal.Bytes())
	require.NoError(t, err)

	publishDealParams = markettypes.PublishStorageDealsParams{
		Deals: []markettypes.ClientDealProposal{{
			Proposal: dealProposal,
			ClientSignature: crypto.Signature{
				Type: crypto.SigTypeBLS,
				Data: sig.Data,
			},
		}},
	}

	serializedParams = new(bytes.Buffer)
	require.NoError(t, publishDealParams.MarshalCBOR(serializedParams))

	m, err = clientApi.MpoolPushMessage(ctx, &types.Message{
		To:     builtin.StorageMarketActorAddr,
		From:   testMiner.OwnerKey.Address,
		Value:  types.FromFil(0),
		Method: builtin.MethodsMarket.PublishStorageDeals,
		Params: serializedParams.Bytes(),
	}, nil)
	require.NoError(t, err)

	r, err = clientApi.StateWaitMsg(ctx, m.Cid(), 2, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.True(t, r.Receipt.ExitCode.IsSuccess())

	// Confirm balance was used

	currTs, err = clientApi.ChainHead(ctx)
	require.NoError(t, err)

	newStateTree, err = state.LoadStateTree(ctxStore, currTs.Blocks()[0].ParentStateRoot)
	require.NoError(t, err)

	datacapAct, err = newStateTree.GetActor(builtin.DatacapActorAddr)
	require.NoError(t, err)

	datacapSt, err = datacap.Load(ctxStore, datacapAct)
	require.NoError(t, err)

	ok, dcap, err = datacapSt.VerifiedClientDataCap(verifiedClientIDAddr)
	require.NoError(t, err)

	require.True(t, ok)
	require.Equal(t, big.Sub(datacapToAssign, big.NewIntUnsigned(uint64(dealProposal.PieceSize)*2)), dcap)

	// The new deal's datacap now belongs to the verifreg
	ok, dcap, err = datacapSt.VerifiedClientDataCap(builtin.VerifiedRegistryActorAddr)
	require.NoError(t, err)

	require.True(t, ok)
	require.Equal(t, big.NewIntUnsigned(uint64(dealProposal.PieceSize)), dcap)

	// now let's make sure we can still remove datacap

	removeProposal := verifregst.RemoveDataCapProposal{
		VerifiedClient: verifiedClientIDAddr,
		// TAKE IT ALL AWAY!
		DataCapAmount:     datacapToAssign,
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
		VerifiedClientToRemove: verifiedClientIDAddr,
		DataCapAmountToRemove:  datacapToAssign,
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
	require.NoError(t, err)

	r, err = clientApi.StateWaitMsg(ctx, m.Cid(), 2, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.True(t, r.Receipt.ExitCode.IsSuccess())

	dc, err = clientApi.StateVerifiedClientStatus(ctx, verifiedClientIDAddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Nil(t, dc)
}

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

func TestMigrationNV19(t *testing.T) {
	kit.QuietMiningLogs()

	blockTime := 5 * time.Millisecond
	nv19epoch := abi.ChainEpoch(100)
	nv20epoch := nv19epoch + builtin.EpochsInDay
	testClient, testMiner, ens := kit.EnsembleMinimal(t, kit.MockProofs(),
		kit.UpgradeSchedule(stmgr.Upgrade{
			Network: network.Version18,
			Height:  -1,
		}, stmgr.Upgrade{
			Network:   network.Version19,
			Height:    nv19epoch,
			Migration: filcns.UpgradeActorsV11,
		}, stmgr.Upgrade{
			Network:   network.Version20,
			Height:    nv20epoch,
			Migration: nil,
		},
		))

	ens.InterconnectAll().BeginMining(blockTime)

	clientApi := testClient.FullNode.(*impl.FullNodeAPI)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testClient.WaitTillChain(ctx, kit.HeightAtLeast(nv19epoch+5))

	bs := blockstore.NewAPIBlockstore(testClient)
	ctxStore := gstStore.WrapBlockStore(ctx, bs)

	postMigrationTs, err := clientApi.ChainHead(ctx)
	require.NoError(t, err)

	newStateTree, err := state.LoadStateTree(ctxStore, postMigrationTs.Blocks()[0].ParentStateRoot)
	require.NoError(t, err)

	require.Equal(t, types.StateTreeVersion5, newStateTree.Version())

	// Now that we have upgraded, we need to check that:

	// - a PoSt is successfully submitted in nv19
	// - a PoSt is successfully submitted in nv20
	// - all claims in the Power actor are of v1_1 type
	// - the miner's info has been updated to the v1_1 type

	// Wait for an nv19 PoSt

	mi, err := testClient.StateMinerInfo(ctx, testMiner.ActorAddr, types.EmptyTSK)
	require.NoError(t, err)

	wact19, err := testClient.StateGetActor(ctx, mi.Worker, types.EmptyTSK)
	require.NoError(t, err)
	en19 := wact19.Nonce

	// wait for a new message to be sent from worker address, it will be a PoSt

waitForProof19:
	for {
		//stm: @CHAIN_STATE_GET_ACTOR_001
		wact, err := testClient.StateGetActor(ctx, mi.Worker, types.EmptyTSK)
		require.NoError(t, err)
		if wact.Nonce > en19 {
			break waitForProof19
		}

		build.Clock.Sleep(blockTime)
	}

	slm19, err := testClient.StateListMessages(ctx, &api.MessageMatch{To: testMiner.ActorAddr}, types.EmptyTSK, 0)
	require.NoError(t, err)

	pmr19, err := testClient.StateSearchMsg(ctx, types.EmptyTSK, slm19[0], -1, false)
	require.NoError(t, err)

	nv19, err := testClient.StateNetworkVersion(ctx, pmr19.TipSet)
	require.NoError(t, err)
	require.Equal(t, network.Version19, nv19)

	require.True(t, pmr19.Receipt.ExitCode.IsSuccess())

	slmsg19, err := testClient.ChainGetMessage(ctx, slm19[0])
	require.NoError(t, err)

	var params19 miner11.SubmitWindowedPoStParams
	require.NoError(t, params19.UnmarshalCBOR(bytes.NewBuffer(slmsg19.Params)))
	require.Equal(t, abi.RegisteredPoStProof_StackedDrgWindow2KiBV1_1, params19.Proofs[0].PoStProof)

	// Wait for nv20

	testClient.WaitTillChain(ctx, kit.HeightAtLeast(nv20epoch+5))

	// Wait for an nv20 PoSt

	wact20, err := testClient.StateGetActor(ctx, mi.Worker, types.EmptyTSK)
	require.NoError(t, err)
	en20 := wact20.Nonce

	// wait for a new message to be sent from worker address, it will be a PoSt

waitForProof20:
	for {
		//stm: @CHAIN_STATE_GET_ACTOR_001
		wact, err := testClient.StateGetActor(ctx, mi.Worker, types.EmptyTSK)
		require.NoError(t, err)
		if wact.Nonce > en20 {
			break waitForProof20
		}

		build.Clock.Sleep(blockTime)
	}

	slm20, err := testClient.StateListMessages(ctx, &api.MessageMatch{To: testMiner.ActorAddr}, types.EmptyTSK, 0)
	require.NoError(t, err)

	pmr20, err := testClient.StateSearchMsg(ctx, types.EmptyTSK, slm20[0], -1, false)
	require.NoError(t, err)

	nv20, err := testClient.StateNetworkVersion(ctx, pmr20.TipSet)
	require.NoError(t, err)
	require.Equal(t, network.Version20, nv20)

	require.True(t, pmr20.Receipt.ExitCode.IsSuccess())

	slmsg20, err := testClient.ChainGetMessage(ctx, slm20[0])
	require.NoError(t, err)

	var params20 miner11.SubmitWindowedPoStParams
	require.NoError(t, params20.UnmarshalCBOR(bytes.NewBuffer(slmsg20.Params)))
	require.Equal(t, abi.RegisteredPoStProof_StackedDrgWindow2KiBV1_1, params20.Proofs[0].PoStProof)

	// check claims in the Power actor

	powerActor, err := newStateTree.GetActor(builtin.StoragePowerActorAddr)
	require.NoError(t, err)

	var powerSt power11.State
	require.NoError(t, ctxStore.Get(ctx, powerActor.Head, &powerSt))

	powerClaims, err := adt11.AsMap(ctxStore, powerSt.Claims, builtin.DefaultHamtBitwidth)
	require.NoError(t, err)

	var claim power11.Claim
	require.NoError(t, powerClaims.ForEach(&claim, func(key string) error {
		v1proof, err := claim.WindowPoStProofType.ToV1_1PostProof()
		require.NoError(t, err)

		require.Equal(t, v1proof, claim.WindowPoStProofType)
		return nil
	}))

	// check MinerInfo

	minerInfo, err := testClient.StateMinerInfo(ctx, testMiner.ActorAddr, types.EmptyTSK)
	require.NoError(t, err)

	v1proof, err := minerInfo.WindowPoStProofType.ToV1_1PostProof()
	require.NoError(t, err)

	require.Equal(t, v1proof, minerInfo.WindowPoStProofType)

}

func TestMigrationNV21(t *testing.T) {
	kit.QuietMiningLogs()

	nv21epoch := abi.ChainEpoch(100)
	testClient, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(),
		kit.UpgradeSchedule(stmgr.Upgrade{
			Network: network.Version20,
			Height:  -1,
		}, stmgr.Upgrade{
			Network:   network.Version21,
			Height:    nv21epoch,
			Migration: filcns.UpgradeActorsV12,
		},
		))

	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	clientApi := testClient.FullNode.(*impl.FullNodeAPI)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testClient.WaitTillChain(ctx, kit.HeightAtLeast(nv21epoch+5))

	// Now that we have upgraded, we need to verify:
	// - Sector info changes executed successfully
	// - Direct data onboarding correct

	bs := blockstore.NewAPIBlockstore(testClient)
	ctxStore := gstStore.WrapBlockStore(ctx, bs)

	currTs, err := clientApi.ChainHead(ctx)
	require.NoError(t, err)

	newStateTree, err := state.LoadStateTree(ctxStore, currTs.Blocks()[0].ParentStateRoot)
	require.NoError(t, err)

	require.Equal(t, types.StateTreeVersion5, newStateTree.Version())

	// check the system actor
	systemAct, err := newStateTree.GetActor(builtin.SystemActorAddr)
	require.NoError(t, err)

	systemCode, ok := actors.GetActorCodeID(actorstypes.Version12, manifest.SystemKey)
	require.True(t, ok)

	require.Equal(t, systemCode, systemAct.Code)

	systemSt, err := system.Load(ctxStore, systemAct)
	require.NoError(t, err)

	manifest12Cid, ok := actors.GetManifest(actorstypes.Version12)
	require.True(t, ok)

	manifest12, err := actors.LoadManifest(ctx, manifest12Cid, ctxStore)
	require.NoError(t, err)
	require.Equal(t, manifest12.Data, systemSt.GetBuiltinActors())

	// start post migration checks

	//todo @aayush sector info changes

	//todo @zen Direct data onboarding tests

}
