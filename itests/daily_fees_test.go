package itests

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	verifreg14 "github.com/filecoin-project/go-state-types/builtin/v14/verifreg"
	miner15 "github.com/filecoin-project/go-state-types/builtin/v15/miner"
	miner16 "github.com/filecoin-project/go-state-types/builtin/v16/miner"
	"github.com/filecoin-project/go-state-types/builtin/v8/util/adt"
	"github.com/filecoin-project/go-state-types/network"
	gstStore "github.com/filecoin-project/go-state-types/store"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/must"
)

func TestDailyFees(t *testing.T) {
	/*** Types & consts *****************************************************************************/

	const defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
	var (
		blocktime = 2 * time.Millisecond
		client    kit.TestFullNode
		genminer  kit.TestMiner
		// don't upgrade until original sectors are fully proven and power updated
		nv25epoch abi.ChainEpoch = builtin.EpochsInDay + 200         // Teep
		nv26epoch abi.ChainEpoch = nv25epoch + builtin.EpochsInDay/2 // Tock
		feePostWg sync.WaitGroup
	)
	// itests start off with a FilReserved of 300M FIL, leading to a circulating supply of 0 initially,
	// but we'll also simulate the calibnet bump of the InitialFilReserved to get the circulating
	// supply calculation up to ~700M FIL. It has ~300M in it, we need to pretend 700M has been used,
	// so 1B - 300M = 700M.
	originalUpgradeTeepInitialFilReserved := buildconstants.UpgradeTeepInitialFilReserved
	buildconstants.UpgradeTeepInitialFilReserved = types.MustParseFIL("1000000000 FIL").Int
	t.Cleanup(func() {
		buildconstants.UpgradeTeepInitialFilReserved = originalUpgradeTeepInitialFilReserved
	})

	type sectorInfo struct {
		sn          abi.SectorNumber
		powerMul    int64
		feeEpoch    abi.ChainEpoch
		expectedFee abi.TokenAmount
	}
	toSectorNumbers := func(si []*sectorInfo) []abi.SectorNumber {
		var sns []abi.SectorNumber
		for _, s := range si {
			sns = append(sns, s.sn)
		}
		return sns
	}
	toExpectedQap := func(si []*sectorInfo) uint64 {
		var power abi.StoragePower
		for _, s := range si {
			power = big.Add(power, abi.NewStoragePower(int64(defaultSectorSize)*s.powerMul))
		}
		return power.Uint64()
	}
	toExpectedRbp := func(si []*sectorInfo) uint64 {
		return uint64(defaultSectorSize) * uint64(len(si))
	}

	/*** Setup **************************************************************************************/

	req := require.New(t)

	kit.QuietMiningLogs()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	initialBigBalance := types.MustParseFIL("100fil").Int64()
	sealProofType := must.One(miner.SealProofTypeFromSectorSize(defaultSectorSize, network.Version23, miner.SealProofVariant_Standard))
	rootKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifierKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifiedClientKey := must.One(key.GenerateKey(types.KTBLS))
	unverifiedClient := must.One(key.GenerateKey(types.KTBLS))
	piece := abi.PieceInfo{Size: abi.PaddedPieceSize(defaultSectorSize), PieceCID: kit.BogusPieceCid2}

	t.Log("*** Setting up network with genesis miner and clients")

	// Setup and begin mining with a single miner (A)
	// Miner A will only be a genesis Miner with power allocated in the genesis block and will not
	// onboard any sectors from here on
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
		}, stmgr.Upgrade{
			Network: network.Version26,
			Height:  nv26epoch,
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

	/*** Utility functions **************************************************************************/

	// circulatingSupplyBefore gets the circulating supply just before the given tipset key;
	// for calculating the fee we need to know the circulating supply that was given to builtin
	// actors at the time it was originally calculated, so we need the CS from the tipset before
	// the one where the message was executed.
	circulatingSupplyBefore := func(tsk types.TipSetKey) abi.TokenAmount {
		ts, err := client.ChainGetTipSet(ctx, tsk)
		req.NoError(err)
		cs, err := client.StateVMCirculatingSupplyInternal(ctx, ts.Parents())
		req.NoError(err)
		return cs.FilCirculating
	}

	// onboardSectors is a wrapper for OnboardSectors that also collects our fee expectations
	onboardSectors := func(sectorBatch *kit.SectorBatch, fees bool, powerMul int64) []*sectorInfo {
		sectorNumbers, tsk := mminer.OnboardSectors(sealProofType, sectorBatch)
		ts, err := client.ChainGetTipSet(ctx, tsk)
		req.NoError(err)
		expectedFee := big.Zero()
		if fees {
			expectedFee = miner16.DailyProofFee(circulatingSupplyBefore(tsk), abi.NewStoragePower(int64(defaultSectorSize)*powerMul))
		}
		var sis []*sectorInfo
		for _, sn := range sectorNumbers {
			sis = append(sis, &sectorInfo{sn, powerMul, ts.Height(), expectedFee})
		}
		return sis
	}

	// snapDeal is a wrapper for SnapDeal that also collects our fee expectations
	snapDeal := func(sector *sectorInfo, manifest kit.SectorManifest, fees bool, powerMul int64) {
		_, tsk := mminer.SnapDeal(sector.sn, manifest)
		if fees {
			sector.expectedFee = miner16.DailyProofFee(circulatingSupplyBefore(tsk), abi.NewStoragePower(int64(defaultSectorSize)*powerMul))
		}
		sector.powerMul = powerMul
		ts, err := client.ChainGetTipSet(ctx, tsk)
		req.NoError(err)
		sector.feeEpoch = ts.Height()
	}

	// extendSector is a wrapper for ExtendSectorExpiration that also collects our fee expectations
	extendSector := func(sector *sectorInfo, extraDuration abi.ChainEpoch, fees bool) {
		soci, err := client.StateSectorGetInfo(ctx, mminer.ActorAddr, sector.sn, types.EmptyTSK)
		req.NoError(err)
		extensionTsk := mminer.ExtendSectorExpiration(sector.sn, soci.Expiration+extraDuration)
		if fees {
			sector.expectedFee = miner16.DailyProofFee(circulatingSupplyBefore(extensionTsk), abi.NewStoragePower(int64(defaultSectorSize)))
		}
		ts, err := client.ChainGetTipSet(ctx, extensionTsk)
		req.NoError(err)
		sector.feeEpoch = ts.Height()
	}

	// checkMiner16Invariants checks the v16 invariants of the miner actor's state
	checkMiner16Invariants := func() {
		act, err := client.StateGetActor(ctx, mminer.ActorAddr, types.EmptyTSK)
		req.NoError(err)
		var st miner16.State
		req.NoError(store.Get(ctx, act.Head, &st))
		_, msgs := miner16.CheckStateInvariants(&st, store, act.Balance)
		req.Len(msgs.Messages(), 0)
	}

	// checkDailyFee checks the daily fee for a sector, returning true if the sector is in the v16
	// format and false if it is in the v15 format. It also returns the daily fee.
	checkDailyFee := func(sector *sectorInfo) (bool, abi.TokenAmount) {
		head, err := client.ChainHead(ctx)
		req.NoError(err)

		st, err := state.LoadStateTree(store, head.ParentState())
		req.NoError(err)

		act, err := st.GetActor(mminer.ActorAddr)
		req.NoError(err)

		var sectorsArr *adt.Array
		{
			nv, err := client.StateNetworkVersion(ctx, head.Key())
			req.NoError(err)
			switch nv {
			case network.Version24:
				var miner miner15.State
				err = store.Get(ctx, act.Head, &miner)
				req.NoError(err)
				sectorsArr, err = adt.AsArray(store, miner.Sectors, miner15.SectorsAmtBitwidth)
				req.NoError(err)
			case network.Version25, network.Version26:
				var miner miner16.State
				err = store.Get(ctx, act.Head, &miner)
				req.NoError(err)
				sectorsArr, err = adt.AsArray(store, miner.Sectors, miner16.SectorsAmtBitwidth)
				req.NoError(err)
			default:
				t.Fatalf("unexpected network version: %d", nv)
			}
		}

		// SectorOnChainInfo has a lazy migration for v16, it could take either a 15 field format or a
		// 16 field format with a DailyFee field on the end. We want to determine whether its a 15 or a
		// 16 field version by first trying to decode it as a 15 field version.

		dailyFee := big.Zero()
		var v16 bool

		var soci15 miner15.SectorOnChainInfo
		ok, err := sectorsArr.Get(uint64(sector.sn), &soci15)
		if err == nil {
			req.True(ok)
		} else {
			// try for v16 sector format, the unmarshaller can also handle the 15 field variety so we do
			// this second
			var soci16 miner16.SectorOnChainInfo
			ok, err = sectorsArr.Get(uint64(sector.sn), &soci16)
			req.NoError(err)
			req.True(ok)
			req.NotNil(soci16.DailyFee)
			req.NotNil(soci16.DailyFee.Int)
			dailyFee = soci16.DailyFee
			v16 = true
		}

		// call the public API and check that it shows what we know
		s, err := client.StateSectorGetInfo(ctx, mminer.ActorAddr, sector.sn, head.Key())
		req.NoError(err)
		req.NotNil(s.DailyFee)
		req.NotNil(s.DailyFee.Int)
		req.Equal(0, big.Cmp(dailyFee, s.DailyFee),
			"daily fees not equal: expected %s, got %s", dailyFee, s.DailyFee)

		return v16, dailyFee
	}

	// checkDailyFeeHasnt checks a sector against its expected fee and asserts it is in v15 format
	checkDailyFeeHasnt := func(sector ...*sectorInfo) {
		for _, s := range sector {
			has, fee := checkDailyFee(s)
			req.False(has)
			req.Equal(big.Zero(), fee)
		}
	}
	// checkDailyFeeHas checks a sector against its expected fee and asserts it is in v16 format
	checkDailyFeeHas := func(sector ...*sectorInfo) {
		for _, s := range sector {
			has, fee := checkDailyFee(s)
			req.True(has)
			req.Equal(s.expectedFee, fee, "sector #%d expected fee %s, got %s", s.sn, s.expectedFee, fee)
		}
	}

	// expectMinerBurn asserts that the miner actor has spent the given amount of funds.
	// UnmanagedMiner generously gives a large Value in its message submissions to the miner actor, so
	// we need to check the miner's balance to see what it currently is, then find all messages sent
	// to the miner actor and sum their values to see how much should be in there. Then the remainder
	// is the amount that has been burned.
	//
	// This is not quite how we expect the daily fee mechanism to work in practice: UnmanagedMiner
	// doesn't (currently) win any block rewards, so it has no vesting funds to pull from. So instead,
	// our daily fee is extracted from the balance. If we had vesting funds, we would need to check
	// that as well.
	expectMinerBurn := func(expectBurn abi.TokenAmount) {
		ts, err := client.ChainHead(ctx)
		req.NoError(err)

		// current balance of the actor
		act, err := client.StateGetActor(ctx, mminer.ActorAddr, types.EmptyTSK)
		req.NoError(err)
		minerBalance := act.Balance

		// hunt through the chain and look for messages sent to our miner and extract their value
		totalSend := big.Zero()
		parentTs := ts.Parents()
		req.NoError(err)
		for ts.Height() > 1 {
			msgs, err := client.ChainGetMessagesInTipset(ctx, parentTs)
			req.NoError(err)
			for _, msg := range msgs {
				if msg.Message.To == mminer.ActorAddr {
					totalSend = big.Add(totalSend, msg.Message.Value)
				}
			}
			ts, err = client.ChainGetTipSet(ctx, parentTs)
			req.NoError(err)
			parentTs = ts.Parents()
		}

		require.Equal(
			t,
			expectBurn.Int64(),
			big.Sub(totalSend, minerBalance).Int64(),
			"expected miner to have burned %s, but it's burned %s", expectBurn, big.Sub(totalSend, minerBalance),
		)
	}

	// checkFeeRecords checks that the DailyFee values in the miner's Deadlines and the FeeDeduction
	// totals in the ExpirationQueues for each partition are consistent with what the public API
	// reports for sector locations and their daily fees.
	checkFeeRecords := func(expectTotalFees abi.TokenAmount) {
		t.Log("*** Check consistency of deadline fee records and expiration queue fee deduction records")

		// deadline DailyFee values should sum up the live sectors in that deadline
		sectors, err := client.StateMinerSectors(ctx, mminer.ActorAddr, nil, types.EmptyTSK)
		req.NoError(err)
		deadlines, err := client.StateMinerDeadlines(ctx, mminer.ActorAddr, types.EmptyTSK)
		req.NoError(err)
		expectedDeadlineFees := make([]abi.TokenAmount, len(deadlines))
		expectedExpirationQueueFees := make(map[miner.SectorLocation]abi.TokenAmount, len(deadlines))
		var actualTotalFees abi.TokenAmount
		for _, sector := range sectors {
			loc, err := client.StateSectorPartition(ctx, mminer.ActorAddr, sector.SectorNumber, types.EmptyTSK)
			req.NoError(err)
			expectedDeadlineFees[loc.Deadline] = big.Add(expectedDeadlineFees[loc.Deadline], sector.DailyFee)
			expectedExpirationQueueFees[*loc] = big.Add(expectedExpirationQueueFees[*loc], sector.DailyFee)
			actualTotalFees = big.Add(actualTotalFees, sector.DailyFee)
		}

		req.Equal(0, big.Cmp(expectTotalFees, actualTotalFees), "expected total fees %s, got %s", expectTotalFees, actualTotalFees)

		// public API has DailyFee on each Deadline
		for i, deadline := range deadlines {
			fee := expectedDeadlineFees[i]
			req.Equal(0, big.Cmp(fee, deadline.DailyFee), "expected %s, got %s for deadline %d", fee, deadline.DailyFee, i)
		}

		nv, err := client.StateNetworkVersion(ctx, types.EmptyTSK)
		req.NoError(err)
		if nv < network.Version25 {
			// nothing to see here, we're done
			return
		}

		// we need to go deeper for the queues
		act, err := client.StateGetActor(ctx, mminer.ActorAddr, types.EmptyTSK)
		req.NoError(err)
		var minerState miner16.State
		err = store.Get(ctx, act.Head, &minerState)
		req.NoError(err)
		dls, err := minerState.LoadDeadlines(store)
		req.NoError(err)
		var checkedExpirationQueueFees int

		err = dls.ForEach(store, func(dlIdx uint64, dl *miner16.Deadline) error {
			// check the daily fee again (sanity check)
			fee := expectedDeadlineFees[dlIdx]
			req.Equal(0, big.Cmp(fee, dl.DailyFee), "expected %s, got %s for deadline %d", fee, dl.DailyFee, dlIdx)

			// dive into the partitions
			partitions, err := dl.PartitionsArray(store)
			req.NoError(err)
			quant := minerState.QuantSpecForDeadline(dlIdx)
			var partition miner16.Partition
			err = partitions.ForEach(&partition, func(i int64) error {
				expectedFee, ok := expectedExpirationQueueFees[miner.SectorLocation{Deadline: dlIdx, Partition: uint64(i)}]
				if ok {
					checkedExpirationQueueFees++
				} else {
					// we don't have record of a sector in here, there shouldn't be a reason to go deeper but
					// just in case ...
					expectedFee = big.Zero()
				}
				var actualFee abi.TokenAmount

				// hunt through the expiration queue and find all the fees that are set to be deducted for
				// this deadline+partition
				queue, err := miner16.LoadExpirationQueue(store, partition.ExpirationsEpochs, quant, miner16.PartitionExpirationAmtBitwidth)
				req.NoError(err)
				var expSet miner16.ExpirationSet
				err = queue.ForEach(&expSet, func(i int64) error {
					actualFee = big.Add(actualFee, expSet.FeeDeduction)
					return nil
				})
				req.NoError(err)

				req.Equal(0, big.Cmp(expectedFee, actualFee), "expected %s, got %s for deadline %d partition %d expiration fee deduction", expectedFee, actualFee, dlIdx, i)
				return nil
			})
			req.NoError(err)

			return nil
		})
		req.NoError(err)

		req.Equal(len(expectedExpirationQueueFees), checkedExpirationQueueFees, "checked %d expiration queue fees, expected %d", checkedExpirationQueueFees, len(expectedExpirationQueueFees))
	}

	// provingWindowsSince calculates how many proving windows have passed for a sector since the
	// specified epoch.
	provingWindowsSince := func(sn abi.SectorNumber, sinceEpoch abi.ChainEpoch) uint64 {
		sectorLoc, err := client.StateSectorPartition(ctx, mminer.ActorAddr, sn, types.EmptyTSK)
		req.NoError(err)
		dlInfo, err := client.StateMinerProvingDeadline(ctx, mminer.ActorAddr, types.EmptyTSK)
		req.NoError(err)

		windowEndOffset := dlInfo.PeriodStart + dlInfo.WPoStChallengeWindow*abi.ChainEpoch(sectorLoc.Deadline+1)
		if windowEndOffset > dlInfo.CurrentEpoch {
			windowEndOffset -= dlInfo.WPoStProvingPeriod // rewind a day
		}
		return uint64((windowEndOffset-sinceEpoch)/dlInfo.WPoStProvingPeriod + 1)
	}

	/*** Test proper starts here ********************************************************************/

	ens.Start()

	expectMinerBurn(big.Zero())
	checkFeeRecords(big.Zero())

	_, verifiedClientAddresses := kit.SetupVerifiedClients(ctx, t, &client, rootKey, verifierKey, []*key.Key{verifiedClientKey})
	verifiedClientAddr := verifiedClientAddresses[0]
	minerId := must.One(address.IDFromAddress(mminer.ActorAddr))

	var allSectors []*sectorInfo

	t.Log("*** Onboarding sectors before the network upgrade")

	// 4 CC sectors, 2 with shorter expirations so we can mess with extensions
	var shortDuration abi.ChainEpoch = miner16.MinSectorExpiration + 100
	ccSectors24 := onboardSectors(
		kit.NewSectorBatch().AddEmptySectors(2).AddSector(kit.SectorManifest{Duration: shortDuration}).AddSector(kit.SectorManifest{Duration: shortDuration}),
		false,
		1,
	)
	allSectors = append(allSectors, ccSectors24...)

	dealSector24 := onboardSectors(kit.NewSectorBatch().AddSectorsWithRandomPieces(1), false, 1)
	allSectors = append(allSectors, dealSector24...)

	clientId, allocationId := kit.SetupAllocation(ctx, t, &client, minerId, piece, verifiedClientAddr, 0, 0)
	verifiedSector24 := onboardSectors(
		kit.NewSectorBatch().AddSector(
			kit.SectorWithVerifiedPiece(piece.PieceCID, &miner14.VerifiedAllocationKey{Client: clientId, ID: verifreg14.AllocationId(allocationId)})),
		false,
		10,
	)
	allSectors = append(allSectors, verifiedSector24...)

	blockMiner.WatchMinerForPost(mminer.ActorAddr)

	t.Log("*** Checking daily fees on sectors onboarded before the network upgrade, before their first PoST")

	// No fees, no fee information at all in these sectors and they are v15 format (sanity check)
	checkDailyFeeHasnt(allSectors...)
	expectMinerBurn(big.Zero())
	checkFeeRecords(big.Zero())

	t.Log("*** Waiting for PoST for sectors onboarded before the network upgrade")

	mminer.WaitTillActivatedAndAssertPower(toSectorNumbers(allSectors), toExpectedRbp(allSectors), toExpectedQap(allSectors))

	t.Log("*** Checking daily fees on sectors onboarded before the network upgrade, after their first PoST")

	// PoST shouldn't have changed anything
	checkDailyFeeHasnt(allSectors...) // still all in v15 format with no fees
	expectMinerBurn(big.Zero())
	checkFeeRecords(big.Zero())

	t.Log("*** Upgrading the network to v25 (Teep)")

	client.WaitTillChain(ctx, kit.HeightAtLeast(nv25epoch+5))
	checkMiner16Invariants()

	t.Log("*** Re-checking daily fees on sectors onboarded before the network upgrade")

	// Still no fees, sectors shouldn't have been touched and still in v15 format
	checkDailyFeeHasnt(allSectors...)
	expectMinerBurn(big.Zero())
	checkFeeRecords(big.Zero())

	t.Log("*** Snapping deals into sectors after the network upgrade")

	// Snap the first two CC sectors, one with an unverified piece, one with a verified piece, capture
	// the CS value at each snap so we can accurately predict the expected daily fee
	snapDeal(ccSectors24[0], kit.SectorManifest{Piece: piece.PieceCID}, true, 1)
	clientId, allocationId = kit.SetupAllocation(ctx, t, &client, minerId, piece, verifiedClientAddr, 0, 0)
	snapDeal(
		ccSectors24[1],
		kit.SectorManifest{Piece: piece.PieceCID, Verified: &miner14.VerifiedAllocationKey{Client: clientId, ID: verifreg14.AllocationId(allocationId)}},
		true,
		10,
	)

	t.Logf("Snapped sectors %d and %d, now have fees: %v & %v", ccSectors24[0].sn, ccSectors24[1].sn, ccSectors24[0].expectedFee, ccSectors24[1].expectedFee)

	cc24PostCount := mminer.GetPostCount(ccSectors24[0].sn) // should be 1, but just in case
	feePostWg.Add(1)
	go func() {
		mminer.WaitTillPostCount(ccSectors24[0].sn, cc24PostCount+1)
		feePostWg.Done()
	}()

	checkMiner16Invariants()

	t.Log("*** Checking daily fees on old and snapped sectors")

	// No fees on untouched sectors still, but because we expect all our sectors to be stored in the
	// root of the HAMT together and we've modified at least one of them, they would have all been
	// rewritten in the new v16 format, so we shouldn't see v15's here anymore.
	checkDailyFeeHas(allSectors...)
	expectMinerBurn(big.Zero()) // fees are registered, but not yet paid
	checkFeeRecords(big.Add(ccSectors24[0].expectedFee, ccSectors24[1].expectedFee))

	t.Log("*** Extending the expiration of a CC sector after the network upgrade")

	extendSector(ccSectors24[2], 30*builtin.EpochsInDay, false)

	// Check that there's still no fees where there shouldn't be - we haven't crossed the grace period
	// for extensions yet
	checkDailyFeeHas(allSectors...)

	t.Log("*** Onboarding sectors after the network upgrade")

	ccSectors25 := onboardSectors(kit.NewSectorBatch().AddEmptySectors(2), true, 1) // 2 CC sectors
	allSectors = append(allSectors, ccSectors25...)
	feePostWg.Add(1)
	go func() {
		mminer.WaitTillPostCount(ccSectors25[0].sn, 1) // onboarded together, they should PoST together
		feePostWg.Done()
	}()

	dealSector25 := onboardSectors(kit.NewSectorBatch().AddSectorsWithRandomPieces(1), true, 1)
	allSectors = append(allSectors, dealSector25...)
	feePostWg.Add(1)
	go func() {
		mminer.WaitTillPostCount(dealSector25[0].sn, 1)
		feePostWg.Done()
	}()

	clientId, allocationId = kit.SetupAllocation(ctx, t, &client, minerId, piece, verifiedClientAddr, 0, 0)
	verifiedSector25 := onboardSectors(
		kit.NewSectorBatch().AddSector(
			kit.SectorWithVerifiedPiece(piece.PieceCID, &miner14.VerifiedAllocationKey{Client: clientId, ID: verifreg14.AllocationId(allocationId)})),
		true,
		10,
	)
	allSectors = append(allSectors, verifiedSector25...)
	feePostWg.Add(1)
	go func() {
		mminer.WaitTillPostCount(verifiedSector25[0].sn, 1)
		feePostWg.Done()
	}()

	// Before PoST
	checkDailyFeeHas(allSectors...)
	checkMiner16Invariants()

	t.Log("*** Waiting for PoST for sectors onboarded after the network upgrade")

	mminer.WaitTillActivatedAndAssertPower(toSectorNumbers(allSectors), toExpectedRbp(allSectors), toExpectedQap(allSectors))

	// After PoST
	checkDailyFeeHas(allSectors...)
	checkMiner16Invariants()

	t.Log("*** Upgrading the network to v26 (Tock)")

	// Move past the upgrade
	client.WaitTillChain(ctx, kit.HeightAtLeast(nv26epoch+5))

	t.Log("*** Extending the expiration of CC sectors after the Tock (v26) upgrade so they attract fees")

	// Extend the one that's already been extended
	extendSector(ccSectors24[2], 30*builtin.EpochsInDay, true)
	// Extend the one that hasn't been extended
	extendSector(ccSectors24[3], 30*builtin.EpochsInDay, true)

	// Check them again
	checkDailyFeeHas(allSectors...)

	// We haven't paid any fees yet, so wait for after the next proving deadline for these sectors
	posts := mminer.GetPostCount(ccSectors24[2].sn)
	feePostWg.Add(1)
	go func() {
		mminer.WaitTillPostCount(ccSectors24[2].sn, posts+1)
		feePostWg.Done()
	}()

	// Wait for all fees to be paid—we need each one to have reached its first deadline and they are
	// likely spread out over multiple deadlines
	feePostWg.Wait()
	// Wait one exta deadline to make sure we get to the end of the current deadline where we've done
	// a PoST
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	client.WaitTillChain(ctx, kit.HeightAtLeast(head.Height()+miner.WPoStChallengeWindow()+10))

	var expectTotalBurn abi.TokenAmount
	for _, sector := range allSectors {
		paymentsPast := provingWindowsSince(sector.sn, sector.feeEpoch)
		expectTotalBurn = big.Add(expectTotalBurn, big.Mul(big.NewInt(int64(paymentsPast)), sector.expectedFee))
	}

	// We've passed our first deadline where fees were payable, both for the snapped nv24 sectors
	// and the nv25 sectors
	expectMinerBurn(expectTotalBurn)
	// With only one fee payment so far for all sectors, the total in the records should be the same
	// as the burn
	checkFeeRecords(expectTotalBurn)
	checkMiner16Invariants()
}
