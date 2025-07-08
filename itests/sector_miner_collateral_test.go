package itests

import (
	"context"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	verifregtypes13 "github.com/filecoin-project/go-state-types/builtin/v13/verifreg"
	datacap2 "github.com/filecoin-project/go-state-types/builtin/v9/datacap"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/datacap"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/proofs"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/must"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
)

func TestMinerBalanceCollateral(t *testing.T) {

	kit.QuietMiningLogs()

	blockTime := 5 * time.Millisecond

	runTest := func(t *testing.T, enabled bool, nSectors int, batching bool) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		opts := kit.ConstructorOpts(
			node.ApplyIf(node.IsType(repo.StorageMiner), node.Override(new(dtypes.GetSealingConfigFunc), func() (dtypes.GetSealingConfigFunc, error) {
				return func() (sealiface.Config, error) {
					cfg := config.DefaultStorageMiner()
					sc := modules.ToSealingConfig(cfg.Dealmaking, cfg.Sealing)

					sc.MaxWaitDealsSectors = 4
					sc.MaxSealingSectors = 4
					sc.MaxSealingSectorsForDeals = 4
					sc.AlwaysKeepUnsealedCopy = true
					sc.WaitDealsDelay = time.Hour

					sc.PreCommitBatchWait = time.Hour
					sc.CommitBatchWait = time.Hour

					sc.MinCommitBatch = nSectors
					sc.MaxPreCommitBatch = nSectors
					sc.MaxCommitBatch = nSectors

					sc.CollateralFromMinerBalance = enabled
					sc.AvailableBalanceBuffer = big.Zero()
					sc.DisableCollateralFallback = false

					return sc, nil
				}, nil
			})),
		)
		full, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(), opts)

		ens.InterconnectAll().BeginMining(blockTime)
		full.WaitTillChain(ctx, kit.HeightAtLeast(10))

		toCheck := miner.StartPledge(ctx, nSectors, 0, nil)

		for len(toCheck) > 0 {
			states := map[api.SectorState]int{}
			for n := range toCheck {
				st, err := miner.StorageMiner.SectorsStatus(ctx, n, false)
				require.NoError(t, err)
				states[st.State]++
				if st.State == api.SectorState(sealing.Proving) {
					delete(toCheck, n)
				}
				if strings.Contains(string(st.State), "Fail") {
					t.Fatal("sector in a failed state", st.State)
				}
			}

			build.Clock.Sleep(100 * time.Millisecond)
		}

		// check that sector messages had zero value set
		sl, err := miner.SectorsListNonGenesis(ctx)
		require.NoError(t, err)

		for _, number := range sl {
			si, err := miner.SectorsStatus(ctx, number, false)
			require.NoError(t, err)

			require.NotNil(t, si.PreCommitMsg)
			pc, err := full.ChainGetMessage(ctx, *si.PreCommitMsg)
			require.NoError(t, err)
			if enabled {
				require.Equal(t, big.Zero(), pc.Value)
			} else {
				require.NotEqual(t, big.Zero(), pc.Value)
			}

			require.NotNil(t, si.CommitMsg)
			c, err := full.ChainGetMessage(ctx, *si.CommitMsg)
			require.NoError(t, err)
			if enabled {
				require.Equal(t, big.Zero(), c.Value)
			}
			// commit value might be zero even with !enabled because in test devnets
			//  precommit deposit tends to be greater than collateral required at
			//  commit time.
		}
	}

	t.Run("nobatch", func(t *testing.T) {
		runTest(t, true, 1, false)
	})
	t.Run("batch-1", func(t *testing.T) {
		runTest(t, true, 1, true) // individual commit instead of aggregate
	})
	t.Run("batch-4", func(t *testing.T) {
		runTest(t, true, 4, true)
	})

	t.Run("nobatch-frombalance-disabled", func(t *testing.T) {
		runTest(t, false, 1, false)
	})
	t.Run("batch-1-frombalance-disabled", func(t *testing.T) {
		runTest(t, false, 1, true) // individual commit instead of aggregate
	})
	t.Run("batch-4-frombalance-disabled", func(t *testing.T) {
		runTest(t, false, 4, true)
	})
}

// TestPledgeCalculations tests the pledge calculations for sectors with different piece combinations
// and verified deals.
// We first verify that the deprecated pledge calculation (StateMinerInitialPledgeCollateral) that
// uses PreCommit with Deal information matches the new one (StateMinerInitialPledgeForSector) that
// just uses sector size, duration and a simple verified size. Post-DDO, we no longer use or need
// the DealIDs field in SectorPreCommitInfo, so the deprecated calculation method can't rely on it
// to determine verified size. In this test, we intentionally build DealIDs so the deprecated method
// still works as expected.
// Then we compare the pledge calculations for sectors with different piece combinations and
// verified deals according to expected rules about verified pieces requiring 10x pledge.
// Most of the complication in this test is for setting up verified deals and allocations so we can
// run the deprecated pledge calculation on a valid precommit.
func TestPledgeCalculations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	kit.QuietMiningLogs()

	blockTime := 5 * time.Millisecond
	rootKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifierKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifiedClientKey := must.One(key.GenerateKey(types.KTBLS))
	oneMFil := must.One(types.ParseFIL("1000000fil"))
	sectorSize := must.One(kit.TestSpt.SectorSize())

	client, testMiner, ens := kit.EnsembleMinimal(
		t,
		kit.MockProofs(),
		kit.RootVerifier(rootKey, abi.NewTokenAmount(oneMFil.Int64())),
		kit.Account(verifierKey, abi.NewTokenAmount(oneMFil.Int64())),
		kit.Account(verifiedClientKey, abi.NewTokenAmount(oneMFil.Int64())),
	)
	ens.Start().InterconnectAll().BeginMining(blockTime)

	testMinerAddr := must.One(testMiner.ActorAddress(ctx))
	testMinerId := abi.ActorID(must.One(address.IDFromAddress(testMinerAddr)))
	testMinerInfo := must.One(client.StateMinerInfo(ctx, testMinerAddr, types.EmptyTSK))
	var verifiedClientAddr address.Address

	// setup verified client
	{
		// import the root key.
		rootAddr, err := client.WalletImport(ctx, &rootKey.KeyInfo)
		require.NoError(t, err)

		// import the verifiers' keys.
		verifierAddr, err := client.WalletImport(ctx, &verifierKey.KeyInfo)
		require.NoError(t, err)

		// import the verified client's key.
		verifiedClientAddr, err = client.WalletImport(ctx, &verifiedClientKey.KeyInfo)
		require.NoError(t, err)

		// fund the verifier
		allowance := big.NewInt(100000000000000)
		params := must.One(actors.SerializeParams(&verifreg.AddVerifierParams{Address: verifierAddr, Allowance: allowance}))
		sm, err := client.MpoolPushMessage(ctx, &types.Message{
			From:   rootAddr,
			To:     verifreg.Address,
			Method: verifreg.Methods.AddVerifier,
			Params: params,
			Value:  big.Zero(),
		}, nil)
		require.NoError(t, err, "AddVerifier failed")

		res, err := client.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
		require.NoError(t, err)
		require.EqualValues(t, 0, res.Receipt.ExitCode)

		verifierAllowance, err := client.StateVerifierStatus(ctx, verifierAddr, types.EmptyTSK)
		require.NoError(t, err)
		require.Equal(t, allowance, *verifierAllowance)

		// assign datacap to client
		params = must.One(actors.SerializeParams(&verifreg.AddVerifiedClientParams{Address: verifiedClientAddr, Allowance: big.NewInt(10000000)}))

		sm, err = client.MpoolPushMessage(ctx, &types.Message{
			From:   verifierAddr,
			To:     verifreg.Address,
			Method: verifreg.Methods.AddVerifiedClient,
			Params: params,
			Value:  big.Zero(),
		}, nil)
		require.NoError(t, err)

		res, err = client.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
		require.NoError(t, err)
		require.EqualValues(t, 0, res.Receipt.ExitCode)
	}

	// we're using mock proofs, so we just need a PieceCID to look like one
	randPieceCid := func() cid.Cid {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		return must.One(commcid.DataCommitmentV1ToCID(b))
	}

	// for verified deals, we need an actual on-chain allocation
	setupAllocation := func(dc abi.PieceInfo) (clientID abi.ActorID, allocationID verifregtypes13.AllocationId) {
		allocationRequest := verifreg.AllocationRequest{
			Provider:   testMinerId,
			Data:       dc.PieceCID,
			Size:       dc.Size,
			TermMin:    verifreg.MinimumVerifiedAllocationTerm,
			TermMax:    verifreg.MaximumVerifiedAllocationTerm,
			Expiration: verifreg.MaximumVerifiedAllocationExpiration,
		}

		receiverParams := must.One(actors.SerializeParams(&verifreg.AllocationRequests{Allocations: []verifreg.AllocationRequest{allocationRequest}}))

		transferParams := must.One(actors.SerializeParams(&datacap2.TransferParams{
			To:           builtin.VerifiedRegistryActorAddr,
			Amount:       big.Mul(big.NewInt(int64(dc.Size)), builtin.TokenPrecision),
			OperatorData: receiverParams,
		}))

		sm, err := client.MpoolPushMessage(ctx, &types.Message{
			To:     builtin.DatacapActorAddr,
			From:   verifiedClientAddr,
			Method: datacap.Methods.TransferExported,
			Params: transferParams,
			Value:  big.Zero(),
		}, nil)
		require.NoError(t, err)

		res, err := client.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
		require.NoError(t, err)
		require.EqualValues(t, 0, res.Receipt.ExitCode)

		// check that we have an allocation
		allocations, err := client.StateGetAllocations(ctx, verifiedClientAddr, types.EmptyTSK)
		require.NoError(t, err)

		for key, value := range allocations {
			if value.Data == dc.PieceCID {
				allocationID = verifregtypes13.AllocationId(key)
				clientID = value.Client
				break
			}
		}
		require.NotEqual(t, verifreg.AllocationId(0), allocationID) // found it in there
		return clientID, allocationID
	}

	head := must.One(client.ChainHead(ctx))

	makeMarketDealProposal := func(data cid.Cid, ps abi.PaddedPieceSize, verified bool) markettypes.ClientDealProposal {
		dp := markettypes.DealProposal{
			PieceCID:             data,
			PieceSize:            ps,
			VerifiedDeal:         verified,
			Client:               verifiedClientAddr,
			Provider:             testMinerAddr,
			Label:                must.One(markettypes.NewLabelFromString("wat")),
			StartEpoch:           head.Height() + 2880*2,
			EndEpoch:             head.Height() + 2880*400,
			StoragePricePerEpoch: big.Zero(),
			ProviderCollateral:   big.Zero(),
			ClientCollateral:     big.Zero(),
		}

		buf := must.One(actors.SerializeParams(&dp))
		sig, err := client.WalletSign(ctx, verifiedClientAddr, buf)
		require.NoError(t, err)

		return markettypes.ClientDealProposal{Proposal: dp, ClientSignature: *sig}
	}

	// make a deal of a given size that may or may not be verified (and require an allocation)
	makeDealOfSize := func(paddedSize int, verified bool) (abi.PieceInfo, abi.DealID) {
		marketPieceSize := abi.PaddedPieceSize(paddedSize)
		pieceData := make([]byte, marketPieceSize)
		_, _ = rand.Read(pieceData)
		pieceCid := randPieceCid()
		pieceInfo := abi.PieceInfo{Size: marketPieceSize, PieceCID: pieceCid}

		if verified {
			setupAllocation(pieceInfo)
		}

		dealProposal := makeMarketDealProposal(pieceCid, marketPieceSize, verified)

		params := must.One(actors.SerializeParams(&markettypes.PublishStorageDealsParams{Deals: []markettypes.ClientDealProposal{dealProposal}}))
		smsg, err := client.MpoolPushMessage(ctx, &types.Message{
			To:     market.Address,
			From:   testMinerInfo.Worker,
			Value:  big.Zero(),
			Method: market.Methods.PublishStorageDeals,
			Params: params,
		}, nil)
		require.NoError(t, err)

		r, err := client.StateWaitMsg(ctx, smsg.Cid(), 1, stmgr.LookbackNoLimit, true)
		require.NoError(t, err)
		require.Equal(t, exitcode.Ok, r.Receipt.ExitCode)

		nv, err := client.StateNetworkVersion(ctx, types.EmptyTSK)
		require.NoError(t, err)
		res, err := market.DecodePublishStorageDealsReturn(r.Receipt.Return, nv)
		require.NoError(t, err)
		dealId := must.One(res.DealIDs())[0]
		return pieceInfo, dealId
	}

	// Setup some sectors with different piece combinations

	type pieceSpec struct {
		size     int
		verified bool
	}
	nextSectorNumber := 22

	sectors := make(map[string]miner.SectorPreCommitInfo)
	pcsb2Params := &miner.PreCommitSectorBatchParams2{Sectors: []miner.SectorPreCommitInfo{}}

	makeSector := func(desc string, pieces []pieceSpec) {
		pieceInfos := make([]abi.PieceInfo, len(pieces))
		dealIds := make([]abi.DealID, len(pieces))
		for i, p := range pieces {
			pieceInfo, deal := makeDealOfSize(p.size, p.verified)
			pieceInfos[i] = pieceInfo
			dealIds[i] = deal
		}

		sector := miner.SectorPreCommitInfo{
			Expiration:    2880 * 800,
			SectorNumber:  abi.SectorNumber(nextSectorNumber),
			SealProof:     kit.TestSpt,
			SealedCID:     cid.MustParse("bagboea4b5abcatlxechwbp7kjpjguna6r6q7ejrhe6mdp3lf34pmswn27pkkiekz"),
			SealRandEpoch: head.Height() - 200,
		}
		nextSectorNumber++

		if len(pieces) > 0 {
			unsealedCid, err := proofs.GenerateUnsealedCID(kit.TestSpt, pieceInfos)
			require.NoError(t, err)
			sector.UnsealedCid = &unsealedCid
			sector.DealIDs = dealIds
		}

		sectors[desc] = sector
		pcsb2Params.Sectors = append(pcsb2Params.Sectors, sector)
	}

	const (
		CC                                   = "CC"
		FullPiece                            = "FullPiece"
		HalfPiece                            = "HalfPiece"
		TwoHalfPieces                        = "TwoHalfPieces"
		FullPieceVerified                    = "FullPieceVerified"
		HalfPieceVerified                    = "HalfPieceVerified"
		TwoHalfPiecesVerified                = "TwoHalfPiecesVerified"
		HalfPieceUnverifiedHalfPieceVerified = "HalfPieceUnverifiedHalfPieceVerified"
	)
	makeSector(CC, nil)
	makeSector(FullPiece, []pieceSpec{{int(sectorSize), false}})
	makeSector(HalfPiece, []pieceSpec{{int(sectorSize) / 2, false}})
	makeSector(TwoHalfPieces, []pieceSpec{{int(sectorSize) / 2, false}, {int(sectorSize) / 2, false}})
	makeSector(FullPieceVerified, []pieceSpec{{int(sectorSize), true}})
	makeSector(HalfPieceVerified, []pieceSpec{{int(sectorSize) / 2, true}})
	makeSector(TwoHalfPiecesVerified, []pieceSpec{{int(sectorSize) / 2, true}, {int(sectorSize) / 2, true}})
	makeSector(HalfPieceUnverifiedHalfPieceVerified, []pieceSpec{{int(sectorSize) / 2, true}, {int(sectorSize) / 2, false}})

	// Submit precommit for all sectors so they are on-chain for our on-chain pledge calculation to access

	params := must.One(actors.SerializeParams(pcsb2Params))
	m, err := client.MpoolPushMessage(ctx, &types.Message{
		To:     testMinerAddr,
		From:   testMiner.OwnerKey.Address,
		Value:  types.FromFil(1),
		Method: builtin.MethodsMiner.PreCommitSectorBatch2,
		Params: params,
	}, nil)
	require.NoError(t, err)

	r, err := client.StateWaitMsg(ctx, m.Cid(), 2, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, r.Receipt.ExitCode)

	tsk := r.TipSet // we're going to perform all pledge calculations at this tipset so we have consistent power, pledge, reward outputs

	verifyPledge := func(sectorNumber abi.SectorNumber, verifiedSize uint64) big.Int {
		// Compare deprecated pledge calculation that uses PreCommit with Deal information with the
		// new one that just uses sector size, duration and a simple verified size. They should be
		// the same.

		precommit, err := client.FullNode.StateSectorPreCommitInfo(ctx, testMinerAddr, sectorNumber, tsk)
		require.NoError(t, err)

		// nolint:staticcheck // SA1019 intentionally using a deprecated method
		pledgeFromPrecommitCC, err := client.FullNode.StateMinerInitialPledgeCollateral(ctx, testMinerAddr, precommit.Info, tsk)
		require.NoError(t, err)
		t.Logf("Pledge from precommit: %s", pledgeFromPrecommitCC)

		pledge, err := client.FullNode.StateMinerInitialPledgeForSector(ctx, precommit.Info.Expiration-r.Height, sectorSize, verifiedSize, tsk)
		require.NoError(t, err)
		t.Logf("Pledge from duration, size and verified: %s", pledge)

		require.Equal(t, pledgeFromPrecommitCC, pledge)

		return pledge
	}

	pledgeCC := verifyPledge(sectors[CC].SectorNumber, 0)
	pledgeFullPiece := verifyPledge(sectors[FullPiece].SectorNumber, 0)
	pledgeHalfPiece := verifyPledge(sectors[HalfPiece].SectorNumber, 0)
	pledgeTwoHalfPieces := verifyPledge(sectors[TwoHalfPieces].SectorNumber, 0)
	pledgeFullPieceVerified := verifyPledge(sectors[FullPieceVerified].SectorNumber, uint64(sectorSize))
	pledgeHalfPieceVerified := verifyPledge(sectors[HalfPieceVerified].SectorNumber, uint64(sectorSize/2))
	pledgeTwoHalfPiecesVerified := verifyPledge(sectors[TwoHalfPiecesVerified].SectorNumber, uint64(sectorSize))
	pledgeHalfPieceUnverifiedHalfPieceVerified := verifyPledge(sectors[HalfPieceUnverifiedHalfPieceVerified].SectorNumber, uint64(sectorSize/2))

	// all sectors without verified pieces should have the same pledge
	require.Equal(t, pledgeCC, pledgeFullPiece)
	require.Equal(t, pledgeCC, pledgeHalfPiece)
	require.Equal(t, pledgeCC, pledgeTwoHalfPieces)

	// fully verified sectors should have the same pledge
	require.Equal(t, pledgeFullPieceVerified, pledgeTwoHalfPiecesVerified)

	// half verified sectors should have the same pledge
	require.Equal(t, pledgeHalfPieceVerified, pledgeHalfPieceUnverifiedHalfPieceVerified)

	// full verified sectors should be 10x CC
	require.Equal(t, big.Mul(big.NewInt(10), pledgeCC), pledgeFullPieceVerified)

	// half verified sectors should be 5x CC + 1/2 CC
	require.Equal(t, big.Add(big.Mul(big.NewInt(5), pledgeCC), big.Div(pledgeCC, big.NewInt(2))), pledgeHalfPieceVerified)
}
