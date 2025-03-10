package itests

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	miner11 "github.com/filecoin-project/go-state-types/builtin/v11/miner"
	power11 "github.com/filecoin-project/go-state-types/builtin/v11/power"
	adt11 "github.com/filecoin-project/go-state-types/builtin/v11/util/adt"
	account14 "github.com/filecoin-project/go-state-types/builtin/v14/account"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	smoothing14 "github.com/filecoin-project/go-state-types/builtin/v14/util/smoothing"
	verifreg14 "github.com/filecoin-project/go-state-types/builtin/v14/verifreg"
	miner15 "github.com/filecoin-project/go-state-types/builtin/v15/miner"
	miner16 "github.com/filecoin-project/go-state-types/builtin/v16/miner"
	"github.com/filecoin-project/go-state-types/builtin/v8/util/adt"
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
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	builtin2 "github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/datacap"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
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
	"github.com/filecoin-project/lotus/lib/must"
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

	res, err = clientApi.StateWaitMsg(ctx, sm.Cid(), 1, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.True(t, res.Receipt.ExitCode.IsSuccess())

	// check datacap balance
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

	spt, err := miner.SealProofTypeFromSectorSize(minerInfo.SectorSize, network.Version17, miner.SealProofVariant_Standard)
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
	ethZeroAddrID, err := newStateTree.LookupIDAddress(ethZeroAddr)
	require.NoError(t, err)
	ethZeroActor, err := newStateTree.GetActor(ethZeroAddrID)
	require.NoError(t, err)
	require.True(t, builtin2.IsEthAccountActor(ethZeroActor.Code))
	require.Equal(t, vm.EmptyObjectCid, ethZeroActor.Head)

	// check all actor's Address fields
	require.NoError(t, newStateTree.ForEach(func(address address.Address, actor *types.Actor) error {
		if address != ethZeroAddrID {
			require.Nil(t, actor.DelegatedAddress)
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

func TestMigrationNV23(t *testing.T) {
	kit.QuietMiningLogs()
	f090Addr, err := address.NewIDAddress(90)
	require.NoError(t, err)
	nv23epoch := abi.ChainEpoch(100)
	testClient, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(),
		kit.UpgradeSchedule(stmgr.Upgrade{
			Network: network.Version22,
			Height:  -1,
		}, stmgr.Upgrade{
			Network:   network.Version23,
			Height:    nv23epoch,
			Migration: filcns.UpgradeActorsV14,
		},
		))

	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	clientApi := testClient.FullNode.(*impl.FullNodeAPI)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testClient.WaitTillChain(ctx, kit.HeightAtLeast(nv23epoch+5))

	bs := blockstore.NewAPIBlockstore(testClient)
	ctxStore := gstStore.WrapBlockStore(ctx, bs)

	preMigrationTs, err := clientApi.ChainGetTipSetByHeight(ctx, nv23epoch-1, types.EmptyTSK)
	require.NoError(t, err)

	root := preMigrationTs.Blocks()[0].ParentStateRoot
	preStateTree, err := state.LoadStateTree(ctxStore, root)
	require.NoError(t, err)
	require.Equal(t, types.StateTreeVersion5, preStateTree.Version())

	// Check f090 actor before migration
	msigCodeNv22, ok := actors.GetActorCodeID(actorstypes.Version13, manifest.MultisigKey)
	assert.True(t, ok)
	f090ActorPre, err := preStateTree.GetActor(f090Addr)
	require.NoError(t, err)
	require.True(t, f090ActorPre.Code.Equals(msigCodeNv22))

	// Get state after the migration
	postMigrationTs, err := clientApi.ChainHead(ctx)
	require.NoError(t, err)
	postStateTree, err := state.LoadStateTree(ctxStore, postMigrationTs.Blocks()[0].ParentStateRoot)
	require.NoError(t, err)

	// Check the new system actor
	systemAct, err := postStateTree.GetActor(builtin.SystemActorAddr)
	require.NoError(t, err)
	systemCode, ok := actors.GetActorCodeID(actorstypes.Version14, manifest.SystemKey)
	require.True(t, ok)
	require.Equal(t, systemCode, systemAct.Code)

	// Check f090 actor after migration
	f090ActorPost, err := postStateTree.GetActor(f090Addr)
	require.NoError(t, err)
	accountNV23, ok := actors.GetActorCodeID(actorstypes.Version14, manifest.AccountKey)
	assert.True(t, ok)
	require.True(t, f090ActorPost.Code.Equals(accountNV23))
	f090StatePost, err := clientApi.StateReadState(ctx, f090Addr, types.EmptyTSK)
	require.NoError(t, err)
	state := f090StatePost.State.(*account14.State)
	require.Equal(t, state.Address, f090Addr)
}

func TestMigrationNV24(t *testing.T) {
	req := require.New(t)

	kit.QuietMiningLogs()

	const (
		nv24epoch               abi.ChainEpoch = 100
		powerRampDurationEpochs uint64         = 200
		blockTime                              = 10 * time.Millisecond
	)
	buildconstants.UpgradeTuktukPowerRampDurationEpochs = powerRampDurationEpochs
	buildconstants.UpgradeTuktukHeight = nv24epoch

	// InitialPledgeMaxPerByte is a little too low for an itest environment so gets in the way of
	// testing the underlying calculation, so we bump it up here so it doesn't interfere.
	miner14.InitialPledgeMaxPerByte = big.Mul(miner14.InitialPledgeMaxPerByte, big.NewInt(10)) // pre migration
	miner15.InitialPledgeMaxPerByte = big.Mul(miner15.InitialPledgeMaxPerByte, big.NewInt(10)) // post migration

	// Observe the rate of change of the pledge calculation during and after the power ramp; change
	// is measured as a difference between the pre-FIP-0081 pledge calculation and the calculation
	// after FIP-0081 has been applied.
	var (
		rateOfChangeDuringRamp []float64
		rateOfChangeAfterRamp  []float64
	)

	testClient, _, ens := kit.EnsembleMinimal(
		t,
		kit.MockProofs(),
		kit.UpgradeSchedule(
			stmgr.Upgrade{
				Network: network.Version23,
				Height:  -1,
			},
			stmgr.Upgrade{
				Network:   network.Version24,
				Height:    nv24epoch,
				Migration: filcns.UpgradeActorsV15,
				PreMigrations: []stmgr.PreMigration{{ // should have no effect on measurements
					PreMigration:    filcns.PreUpgradeActorsV15,
					StartWithin:     nv24epoch / 2,
					DontStartWithin: 2,
					StopWithin:      2,
				}},
				Expensive: true,
			},
		))

	ens.InterconnectAll().BeginMining(blockTime)

	clientApi := testClient.FullNode.(*impl.FullNodeAPI)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Watch the chain and capture the pledge every ~5 epochs to observe the rate of change before,
	// during, and after the FIP-0081 pledge ramp.
	doneCh := make(chan struct{})
	go func() {
		var lastRelativeChange float64
		var lastEpoch abi.ChainEpoch

		start, err := clientApi.ChainHead(ctx)
		assert.NoError(t, err)

		for i := 0; ; i++ {
			head := testClient.WaitTillChain(ctx, kit.HeightAtLeast(start.Height()+abi.ChainEpoch(i*5)))
			if i > 0 && head.Height() == lastEpoch {
				t.Logf("skipping duplicate pledge calculation @%d, slow test runner?", head.Height())
				continue
			}
			lastEpoch = head.Height()

			pledge, err := clientApi.StateMinerInitialPledgeForSector(ctx, abi.ChainEpoch(builtin.EpochsInYear), abi.SectorSize(2<<10), 0, head.Key())
			if err != nil {
				t.Errorf("failed to calculate pledge %d: %s", head.Height(), err)
				break
			}
			preFip0081Pledge := preFip0081StateMinerInitialPledgeForSector(ctx, t, clientApi, abi.ChainEpoch(builtin.EpochsInYear), abi.SectorSize(2<<10), 0, head.Key())

			relativeChange := float64(pledge.Uint64()-preFip0081Pledge.Uint64()) / float64(preFip0081Pledge.Uint64())
			if i > 0 {
				if head.Height() > nv24epoch {
					// We want to see an increasing distance between the pre-FIP-0081 pledge and original pledge
					// calculation, so the relative change should be increasing from the migration onward.
					// This test depends on network power being below baseline, once we are above baseline
					// then the FIP-0081 70/30 split (including ramp) should not differ from the pre-FIP-0081
					// pledge calculation.
					if relativeChange < lastRelativeChange {
						t.Errorf("relative change decreased: %f -> %f @%d", lastRelativeChange, relativeChange, head.Height())
						break
					}
				}

				rateOfChange := relativeChange - lastRelativeChange
				t.Logf("Pledge @%d: %d (pre-fip-0081: %d, rate of change: %f)", head.Height(), pledge.Uint64(), preFip0081Pledge.Uint64(), rateOfChange)
				switch {
				case head.Height() < nv24epoch:
					if rateOfChange != 0 {
						// likely something's wrong with our preFip0081StateMinerInitialPledgeForSector implementation
						t.Errorf("rate of change should be zero before the migration: %f @%d", rateOfChange, head.Height())
						break
					}
				case head.Height() <= nv24epoch+abi.ChainEpoch(powerRampDurationEpochs):
					rateOfChangeDuringRamp = append(rateOfChangeDuringRamp, rateOfChange)
				default:
					rateOfChangeAfterRamp = append(rateOfChangeAfterRamp, rateOfChange)
				}
			}
			lastRelativeChange = relativeChange

			// Observe for another `powerRampDurationEpochs` epochs after the ramp
			if head.Height() > nv24epoch+abi.ChainEpoch(powerRampDurationEpochs*2) {
				break
			}
		}

		close(doneCh)
	}()

	testClient.WaitTillChain(ctx, kit.HeightAtLeast(nv24epoch+5))

	bs := blockstore.NewAPIBlockstore(testClient)
	ctxStore := gstStore.WrapBlockStore(ctx, bs)

	// Get state before the migration
	preMigrationTs, err := clientApi.ChainGetTipSetByHeight(ctx, nv24epoch-1, types.EmptyTSK)
	req.NoError(err)

	root := preMigrationTs.Blocks()[0].ParentStateRoot
	preStateTree, err := state.LoadStateTree(ctxStore, root)
	req.NoError(err)
	req.Equal(types.StateTreeVersion5, preStateTree.Version())

	// FIP-0081 pledge ramp settings are unset before migration
	powerActor, err := preStateTree.GetActor(builtin.StoragePowerActorAddr)
	req.NoError(err)
	powerState, err := power.Load(ctxStore, powerActor)
	req.NoError(err)
	req.Equal(int64(0), powerState.RampStartEpoch())
	req.Equal(uint64(0), powerState.RampDurationEpochs())

	// Get state after the migration
	postMigrationTs, err := clientApi.ChainHead(ctx)
	req.NoError(err)
	postStateTree, err := state.LoadStateTree(ctxStore, postMigrationTs.Blocks()[0].ParentStateRoot)
	req.NoError(err)

	// Check the new system actor
	systemAct, err := postStateTree.GetActor(builtin.SystemActorAddr)
	req.NoError(err)
	systemCode, ok := actors.GetActorCodeID(actorstypes.Version15, manifest.SystemKey)
	req.True(ok)
	req.Equal(systemCode, systemAct.Code)

	// FIP-0081 pledge ramp settings are set after migration
	powerActor, err = postStateTree.GetActor(builtin.StoragePowerActorAddr)
	req.NoError(err)
	powerState, err = power.Load(ctxStore, powerActor)
	req.NoError(err)
	req.Equal(int64(nv24epoch), powerState.RampStartEpoch())
	req.Equal(powerRampDurationEpochs, powerState.RampDurationEpochs())

	// Sanity check our preFip0081StateMinerInitialPledgeForSector calculation is correct for pre-0081
	preMigrationPledge, err := clientApi.StateMinerInitialPledgeForSector(ctx, abi.ChainEpoch(builtin.EpochsInYear), abi.SectorSize(2<<10), 0, preMigrationTs.Key())
	req.NoError(err)
	preFip0081Pledge := preFip0081StateMinerInitialPledgeForSector(ctx, t, clientApi, abi.ChainEpoch(builtin.EpochsInYear), abi.SectorSize(2<<10), 0, preMigrationTs.Key())
	req.Equal(preFip0081Pledge, preMigrationPledge)

	// Wait for the rate of change calculation to complete
	<-doneCh

	average := func(arr []float64) float64 {
		sum := 0.0
		for _, v := range arr {
			sum += v
		}
		return sum / float64(len(arr))
	}

	avgRateOfChangeDuringRamp := average(rateOfChangeDuringRamp)
	avgRateOfChangeAfterRamp := average(rateOfChangeAfterRamp)
	t.Logf("Average rate of change during ramp: %f", avgRateOfChangeDuringRamp)
	t.Logf("Average rate of change after ramp: %f", avgRateOfChangeAfterRamp)
	req.Less(avgRateOfChangeAfterRamp, avgRateOfChangeDuringRamp)
}

// preFip0081StateMinerInitialPledgeForSector is the same calculation as StateMinerInitialPledgeForSector
// but uses miner14's version of the calculation without the FIP-0081 changes.
func preFip0081StateMinerInitialPledgeForSector(ctx context.Context, t *testing.T, client *impl.FullNodeAPI, sectorDuration abi.ChainEpoch, sectorSize abi.SectorSize, verifiedSize uint64, tsk types.TipSetKey) types.BigInt {
	req := require.New(t)

	bs := blockstore.NewAPIBlockstore(client)
	ctxStore := gstStore.WrapBlockStore(ctx, bs)

	ts, err := client.ChainGetTipSet(ctx, tsk)
	req.NoError(err)

	circSupply, err := client.StateVMCirculatingSupplyInternal(ctx, ts.Key())
	req.NoError(err)

	powerActor, err := client.StateGetActor(ctx, power.Address, ts.Key())
	req.NoError(err)

	powerState, err := power.Load(ctxStore, powerActor)
	req.NoError(err)

	rewardActor, err := client.StateGetActor(ctx, reward.Address, ts.Key())
	req.NoError(err)

	rewardState, err := reward.Load(ctxStore, rewardActor)
	req.NoError(err)

	networkQAPower, err := powerState.TotalPowerSmoothed()
	req.NoError(err)

	verifiedWeight := big.Mul(big.NewIntUnsigned(verifiedSize), big.NewInt(int64(sectorDuration)))
	sectorWeight := builtin2.QAPowerForWeight(sectorSize, sectorDuration, verifiedWeight)

	thisEpochBaselinePower, err := rewardState.(interface {
		ThisEpochBaselinePower() (abi.StoragePower, error)
	}).ThisEpochBaselinePower()
	req.NoError(err)
	thisEpochRewardSmoothed, err := rewardState.(interface {
		ThisEpochRewardSmoothed() (builtin2.FilterEstimate, error)
	}).ThisEpochRewardSmoothed()
	req.NoError(err)

	rewardEstimate := smoothing14.FilterEstimate{
		PositionEstimate: thisEpochRewardSmoothed.PositionEstimate,
		VelocityEstimate: thisEpochRewardSmoothed.VelocityEstimate,
	}
	networkQAPowerEstimate := smoothing14.FilterEstimate{
		PositionEstimate: networkQAPower.PositionEstimate,
		VelocityEstimate: networkQAPower.VelocityEstimate,
	}

	initialPledge := miner14.InitialPledgeForPower(
		sectorWeight,
		thisEpochBaselinePower,
		rewardEstimate,
		networkQAPowerEstimate,
		circSupply.FilCirculating,
	)

	var initialPledgeNum = types.NewInt(110)
	var initialPledgeDen = types.NewInt(100)

	return types.BigDiv(types.BigMul(initialPledge, initialPledgeNum), initialPledgeDen)
}

func TestMigrationNV25(t *testing.T) {
	req := require.New(t)

	kit.QuietMiningLogs()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
	var (
		blocktime = 2 * time.Millisecond
		client    kit.TestFullNode
		genminer  kit.TestMiner
		// don't upgrade until our original sectors are fully proven and power updated, to keep the test simple
		nv25epoch abi.ChainEpoch = builtin.EpochsInDay + 200
	)

	initialBigBalance := types.MustParseFIL("100fil").Int64()
	sealProofType := must.One(miner.SealProofTypeFromSectorSize(defaultSectorSize, network.Version23, miner.SealProofVariant_Standard))
	rootKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifierKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifiedClientKey := must.One(key.GenerateKey(types.KTBLS))
	unverifiedClient := must.One(key.GenerateKey(types.KTBLS))

	// Setup and begin mining with a single miner (A)
	// Miner A will only be a genesis Miner with power allocated in the genesis block and will not onboard any sectors from here on
	ens := kit.NewEnsemble(t,
		kit.MockProofs(true),
		kit.RootVerifier(rootKey, abi.NewTokenAmount(initialBigBalance)),
		kit.Account(verifierKey, abi.NewTokenAmount(initialBigBalance)),
		kit.Account(verifiedClientKey, abi.NewTokenAmount(initialBigBalance)),
		kit.Account(unverifiedClient, abi.NewTokenAmount(initialBigBalance)),
		kit.UpgradeSchedule(stmgr.Upgrade{
			Network: network.Version24,
			Height:  -1,
		}, stmgr.Upgrade{
			Network:   network.Version25,
			Height:    nv25epoch,
			Migration: filcns.UpgradeActorsV16,
		},
		)).
		FullNode(&client, kit.SectorSize(defaultSectorSize)).
		Miner(&genminer, &client, kit.PresealSectors(5), kit.SectorSize(defaultSectorSize), kit.WithAllSubsystems()).
		Start().
		InterconnectAll()

	store := gstStore.WrapBlockStore(ctx, blockstore.NewAPIBlockstore(client))

	blockMiners := ens.BeginMiningMustPost(blocktime)
	req.Len(blockMiners, 1)
	blockMiner := blockMiners[0]

	nodeOpts := []kit.NodeOpt{kit.SectorSize(defaultSectorSize), kit.OwnerAddr(client.DefaultKey)}
	mminer, ens := ens.UnmanagedMiner(ctx, &client, nodeOpts...)
	defer mminer.Stop()

	ens.Start()

	// Onboard some sectors before the network upgrade

	_, verifiedClientAddresses := kit.SetupVerifiedClients(ctx, t, &client, rootKey, verifierKey, []*key.Key{verifiedClientKey})
	verifiedClientAddr := verifiedClientAddresses[0]
	minerId := must.One(address.IDFromAddress(mminer.ActorAddr))

	piece := abi.PieceInfo{
		Size:     abi.PaddedPieceSize(defaultSectorSize),
		PieceCID: cid.MustParse("baga6ea4seaqlhznlutptgfwhffupyer6txswamerq5fc2jlwf2lys2mm5jtiaeq"),
	}
	clientId, allocationId := kit.SetupAllocation(ctx, t, &client, minerId, piece, verifiedClientAddr, 0, 0)

	var allSectors []abi.SectorNumber
	ccSectors := mminer.OnboardSectors(sealProofType, []kit.SectorManifest{{}, {}}) // 2 CC sectors
	allSectors = append(allSectors, ccSectors...)
	dealSector := mminer.OnboardSectors(sealProofType, []kit.SectorManifest{kit.SectorWithRandPiece()})
	allSectors = append(allSectors, dealSector...)
	verifiedSector := mminer.OnboardSectors(sealProofType, []kit.SectorManifest{
		{Piece: piece.PieceCID, Verified: &miner14.VerifiedAllocationKey{Client: clientId, ID: verifreg14.AllocationId(allocationId)}}})
	allSectors = append(allSectors, verifiedSector...)

	blockMiner.WatchMinerForPost(mminer.ActorAddr)

	expectedRaw := uint64(defaultSectorSize * 4)  // 4 sectors onboarded
	expectedQap := uint64(defaultSectorSize * 13) // 3 sectors + 1 verified sector
	mminer.WaitTillActivatedAndAssertPower(allSectors, expectedRaw, expectedQap)

	checkDailyFee := func(sn abi.SectorNumber) (bool, abi.TokenAmount) {
		head, err := client.ChainHead(ctx)
		req.NoError(err)

		st, err := state.LoadStateTree(store, head.ParentState())
		require.NoError(t, err)

		act, err := st.GetActor(mminer.ActorAddr)
		require.NoError(t, err)

		var sectorsArr *adt.Array
		{
			nv, err := client.StateNetworkVersion(ctx, head.Key())
			require.NoError(t, err)
			switch nv {
			case network.Version24:
				var miner miner15.State
				err = store.Get(ctx, act.Head, &miner)
				require.NoError(t, err)
				sectorsArr, err = adt.AsArray(store, miner.Sectors, miner15.SectorsAmtBitwidth)
				require.NoError(t, err)
			case network.Version25:
				var miner miner16.State
				err = store.Get(ctx, act.Head, &miner)
				require.NoError(t, err)
				sectorsArr, err = adt.AsArray(store, miner.Sectors, miner16.SectorsAmtBitwidth)
				require.NoError(t, err)
			default:
				t.Fatalf("unexpected network version: %d", nv)
			}
		}

		// SectorOnChainInfo has a lazy migration for v16, it could take either a 15 field format or a
		// 16 field format with a DailyFee field on the end. We want to determine whether its a 15 or a
		// 16 field version by first trying to decode it as a 15 field version.

		var soci15 miner15.SectorOnChainInfo
		ok, err := sectorsArr.Get(uint64(sn), &soci15)
		if err == nil {
			require.True(t, ok)
			return false, abi.NewTokenAmount(0)
		}

		// try for v16 sector format, the unmarshaller can also handle the 15 field variety so we do
		// this second
		var soci16 miner16.SectorOnChainInfo
		ok, err = sectorsArr.Get(uint64(sn), &soci16)
		require.NoError(t, err)
		require.True(t, ok)
		return true, soci16.DailyFee
	}

	// No fees, no fee information at all in these sectors (sanity check)
	for _, sn := range allSectors {
		has, fee := checkDailyFee(sn)
		require.False(t, has) // v15
		require.Equal(t, abi.NewTokenAmount(0), fee)
	}

	// Move past the upgrade
	client.WaitTillChain(ctx, kit.HeightAtLeast(nv25epoch+5))

	// Still no fees, sectors shouldn't have been touched
	for _, sn := range allSectors {
		has, fee := checkDailyFee(sn)
		require.False(t, has) // v15
		require.Equal(t, abi.NewTokenAmount(0), fee)
	}

	// Snap both CC sectors, one with an unverified piece, one with a verified piece, capture the
	// CS value at each snap so we can accurately predict the expected daily fee
	_, snap0Ts := mminer.SnapDeal(ccSectors[0], kit.SectorManifest{Piece: piece.PieceCID})
	snap0CS, err := client.StateVMCirculatingSupplyInternal(ctx, snap0Ts)
	require.NoError(t, err)
	clientId, allocationId = kit.SetupAllocation(ctx, t, &client, minerId, piece, verifiedClientAddr, 0, 0)
	_, snap1Ts := mminer.SnapDeal(ccSectors[1], kit.SectorManifest{Piece: piece.PieceCID, Verified: &miner14.VerifiedAllocationKey{Client: clientId, ID: verifreg14.AllocationId(allocationId)}})
	snap1CS, err := client.StateVMCirculatingSupplyInternal(ctx, snap1Ts)
	require.NoError(t, err)

	// No fees on untouched sectors, but because we expect all our sectors to be stored in the root
	// of the HAMT together and we've modified at least one of them, they would have all been
	// rewritten in the new v16 format, so we shouldn't see v15's here anymore.
	for _, sn := range append(append([]abi.SectorNumber{}, dealSector...), verifiedSector...) {
		has, fee := checkDailyFee(sn)
		require.True(t, has) // v16
		require.Equal(t, abi.NewTokenAmount(0), fee)
	}

	// fees on snapped sectors, first our non-verified sector, then our verified sector, with 10x qap
	has, fee := checkDailyFee(ccSectors[0])
	require.True(t, has)
	require.Equal(t, miner16.DailyProofFee(snap0CS.FilCirculating, abi.NewStoragePower(int64(defaultSectorSize))), fee)
	has, fee = checkDailyFee(ccSectors[1])
	require.True(t, has)
	require.Equal(t, miner16.DailyProofFee(snap1CS.FilCirculating, abi.NewStoragePower(int64(defaultSectorSize*10))), fee)
}
