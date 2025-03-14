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
		feePostWg sync.WaitGroup
	)

	initialBigBalance := types.MustParseFIL("100fil").Int64()
	sealProofType := must.One(miner.SealProofTypeFromSectorSize(defaultSectorSize, network.Version23, miner.SealProofVariant_Standard))
	rootKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifierKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifiedClientKey := must.One(key.GenerateKey(types.KTBLS))
	unverifiedClient := must.One(key.GenerateKey(types.KTBLS))

	t.Log("*** Setting up network with genesis miner and clients")

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
	checkDailyFee := func(sn abi.SectorNumber) (bool, abi.TokenAmount) {
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
				t.Logf("Got miner15.State: %+v", miner)
			case network.Version25:
				var miner miner16.State
				err = store.Get(ctx, act.Head, &miner)
				req.NoError(err)
				sectorsArr, err = adt.AsArray(store, miner.Sectors, miner16.SectorsAmtBitwidth)
				req.NoError(err)
				t.Logf("Got miner16.State: %+v", miner)
			default:
				t.Fatalf("unexpected network version: %d", nv)
			}
		}

		// SectorOnChainInfo has a lazy migration for v16, it could take either a 15 field format or a
		// 16 field format with a DailyFee field on the end. We want to determine whether its a 15 or a
		// 16 field version by first trying to decode it as a 15 field version.

		dailyFee := abi.NewTokenAmount(0)
		var v16 bool

		var soci15 miner15.SectorOnChainInfo
		ok, err := sectorsArr.Get(uint64(sn), &soci15)
		if err == nil {
			req.True(ok)
		} else {
			// try for v16 sector format, the unmarshaller can also handle the 15 field variety so we do
			// this second
			var soci16 miner16.SectorOnChainInfo
			ok, err = sectorsArr.Get(uint64(sn), &soci16)
			req.NoError(err)
			req.True(ok)
			req.NotNil(soci16.DailyFee)
			req.NotNil(soci16.DailyFee.Int)
			dailyFee = soci16.DailyFee
			v16 = true
		}

		// call the public API and check that it shows what we know
		s, err := client.StateSectorGetInfo(ctx, mminer.ActorAddr, sn, head.Key())
		req.NoError(err)
		req.NotNil(s.DailyFee)
		req.NotNil(s.DailyFee.Int)
		req.Equal(0, big.Cmp(dailyFee, s.DailyFee),
			"daily fees not equal: expected %s, got %s", dailyFee, s.DailyFee)

		return v16, dailyFee
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
		totalSend := abi.NewTokenAmount(0)
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
			"expected miner to have burned %s, but it's burned %s", expectBurn, big.Sub(minerBalance, totalSend),
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
			if !fee.IsZero() {
				t.Logf("checked deadline %d daily fee: expected %s, got %s", dlIdx, fee, dl.DailyFee)
			}

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
					// we don't have record of a sector in here, there shouldn't be a reason to go deeper but just in case ...
					expectedFee = abi.NewTokenAmount(0)
				}
				var actualFee abi.TokenAmount

				// hunt through the expiration queue and find all the fees that are set to be deducted for this deadline+partition
				queue, err := miner16.LoadExpirationQueue(store, partition.ExpirationsEpochs, quant, miner16.PartitionExpirationAmtBitwidth)
				req.NoError(err)
				var expSet miner16.ExpirationSet
				err = queue.ForEach(&expSet, func(i int64) error {
					actualFee = big.Add(actualFee, expSet.FeeDeduction)
					return nil
				})
				req.NoError(err)

				req.Equal(0, big.Cmp(expectedFee, actualFee), "expected %s, got %s for deadline %d partition %d expiration fee deduction", expectedFee, actualFee, dlIdx, i)
				t.Logf("checked deadline %d partition %d expiration fee deduction: expected %s, got %s", dlIdx, i, expectedFee, actualFee)
				return nil
			})
			req.NoError(err)

			return nil
		})
		req.NoError(err)

		req.Equal(len(expectedExpirationQueueFees), checkedExpirationQueueFees, "checked %d expiration queue fees, expected %d", checkedExpirationQueueFees, len(expectedExpirationQueueFees))
	}

	ens.Start()

	t.Log("*** Onboarding sectors before the network upgrade")
	expectMinerBurn(abi.NewTokenAmount(0))
	checkFeeRecords(abi.NewTokenAmount(0))

	_, verifiedClientAddresses := kit.SetupVerifiedClients(ctx, t, &client, rootKey, verifierKey, []*key.Key{verifiedClientKey})
	verifiedClientAddr := verifiedClientAddresses[0]
	minerId := must.One(address.IDFromAddress(mminer.ActorAddr))

	piece := abi.PieceInfo{
		Size:     abi.PaddedPieceSize(defaultSectorSize),
		PieceCID: kit.BogusPieceCid2,
	}
	clientId, allocationId := kit.SetupAllocation(ctx, t, &client, minerId, piece, verifiedClientAddr, 0, 0)

	var allSectors []abi.SectorNumber
	ccSectors24, _ := mminer.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(2)) // 2 CC sectors
	allSectors = append(allSectors, ccSectors24...)
	dealSector24, _ := mminer.OnboardSectors(sealProofType, kit.NewSectorBatch().AddSectorsWithRandomPieces(1))
	allSectors = append(allSectors, dealSector24...)
	verifiedSector24, _ := mminer.OnboardSectors(
		sealProofType,
		kit.NewSectorBatch().AddSector(
			kit.SectorWithVerifiedPiece(piece.PieceCID, &miner14.VerifiedAllocationKey{Client: clientId, ID: verifreg14.AllocationId(allocationId)})))
	allSectors = append(allSectors, verifiedSector24...)

	blockMiner.WatchMinerForPost(mminer.ActorAddr)

	t.Log("*** Checking daily fees on sectors onboarded before the network upgrade, before their first PoST")

	// No fees, no fee information at all in these sectors (sanity check)
	for _, sn := range allSectors {
		has, fee := checkDailyFee(sn)
		req.False(has) // v15
		req.Equal(abi.NewTokenAmount(0), fee)
	}
	expectMinerBurn(abi.NewTokenAmount(0))
	checkFeeRecords(abi.NewTokenAmount(0))

	t.Log("*** Waiting for PoST for sectors onboarded before the network upgrade")

	expectedRaw := uint64(defaultSectorSize * 4)  // 4 sectors onboarded
	expectedQap := uint64(defaultSectorSize * 13) // 3 sectors + 1 verified sector
	mminer.WaitTillActivatedAndAssertPower(allSectors, expectedRaw, expectedQap)

	t.Log("*** Checking daily fees on sectors onboarded before the network upgrade, after their first PoST")

	// PoST shouldn't have changed anything
	for _, sn := range allSectors {
		has, fee := checkDailyFee(sn)
		req.False(has) // v15
		req.Equal(abi.NewTokenAmount(0), fee)
	}
	expectMinerBurn(abi.NewTokenAmount(0))
	checkFeeRecords(abi.NewTokenAmount(0))

	// Move past the upgrade
	client.WaitTillChain(ctx, kit.HeightAtLeast(nv25epoch+5))

	checkMiner16Invariants()

	t.Log("*** Re-checking daily fees on sectors onboarded before the network upgrade")

	// Still no fees, sectors shouldn't have been touched
	for _, sn := range allSectors {
		has, fee := checkDailyFee(sn)
		req.False(has) // v15
		req.Equal(abi.NewTokenAmount(0), fee)
	}
	expectMinerBurn(abi.NewTokenAmount(0))
	checkFeeRecords(abi.NewTokenAmount(0))

	t.Log("*** Snapping deals into sectors after the network upgrade")

	// Snap both CC sectors, one with an unverified piece, one with a verified piece, capture the
	// CS value at each snap so we can accurately predict the expected daily fee
	_, snap0Tsk := mminer.SnapDeal(ccSectors24[0], kit.SectorManifest{Piece: piece.PieceCID})
	ccSectors240ExpectedFee := miner16.DailyProofFee(circulatingSupplyBefore(snap0Tsk), abi.NewStoragePower(int64(defaultSectorSize)))
	clientId, allocationId = kit.SetupAllocation(ctx, t, &client, minerId, piece, verifiedClientAddr, 0, 0)
	_, snap1Tsk := mminer.SnapDeal(ccSectors24[1], kit.SectorManifest{Piece: piece.PieceCID, Verified: &miner14.VerifiedAllocationKey{Client: clientId, ID: verifreg14.AllocationId(allocationId)}})
	ccSectors241ExpectedFee := miner16.DailyProofFee(circulatingSupplyBefore(snap1Tsk), abi.NewStoragePower(int64(defaultSectorSize*10)))

	feePostWg.Add(1)
	go func() {
		mminer.WaitTillPost(ccSectors24[0]) // both onboarded together, so they should post together
		feePostWg.Done()
	}()

	checkMiner16Invariants()

	t.Log("*** Checking daily fees on old and snapped sectors")

	// No fees on untouched sectors, but because we expect all our sectors to be stored in the root
	// of the HAMT together and we've modified at least one of them, they would have all been
	// rewritten in the new v16 format, so we shouldn't see v15's here anymore.
	noFeeSectors := append(append([]abi.SectorNumber{}, dealSector24...), verifiedSector24...)
	for _, sn := range noFeeSectors {
		has, fee := checkDailyFee(sn)
		req.True(has) // v16
		req.Equal(abi.NewTokenAmount(0), fee)
	}

	// fees on snapped sectors, first our non-verified sector, then our verified sector, with 10x qap
	has, fee := checkDailyFee(ccSectors24[0])
	req.True(has)
	req.Equal(ccSectors240ExpectedFee, fee)
	has, fee = checkDailyFee(ccSectors24[1])
	req.True(has)
	req.Equal(ccSectors241ExpectedFee, fee)

	expectMinerBurn(abi.NewTokenAmount(0)) // fees are registered, but not yet paid
	checkFeeRecords(big.Add(ccSectors240ExpectedFee, ccSectors241ExpectedFee))

	t.Log("*** Onboarding sectors after the network upgrade")

	clientId, allocationId = kit.SetupAllocation(ctx, t, &client, minerId, piece, verifiedClientAddr, 0, 0)

	ccSectors25, ccTsk := mminer.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(2)) // 2 CC sectors
	allSectors = append(allSectors, ccSectors25...)
	ccSectors25ExpectedFee := miner16.DailyProofFee(circulatingSupplyBefore(ccTsk), abi.NewStoragePower(int64(defaultSectorSize)))
	feePostWg.Add(1)
	go func() {
		mminer.WaitTillPost(ccSectors25[0]) // both onboarded together, so they should post together
		feePostWg.Done()
	}()

	dealSector25, dealTsk := mminer.OnboardSectors(sealProofType, kit.NewSectorBatch().AddSectorsWithRandomPieces(1))
	allSectors = append(allSectors, dealSector25...)
	dealSector25ExpectedFee := miner16.DailyProofFee(circulatingSupplyBefore(dealTsk), abi.NewStoragePower(int64(defaultSectorSize)))
	feePostWg.Add(1)
	go func() {
		mminer.WaitTillPost(dealSector25[0])
		feePostWg.Done()
	}()

	verifiedSector25, verifiedTsk := mminer.OnboardSectors(
		sealProofType,
		kit.NewSectorBatch().AddSector(
			kit.SectorWithVerifiedPiece(piece.PieceCID, &miner14.VerifiedAllocationKey{Client: clientId, ID: verifreg14.AllocationId(allocationId)})))
	allSectors = append(allSectors, verifiedSector25...)
	verified25ExpectedFee := miner16.DailyProofFee(circulatingSupplyBefore(verifiedTsk), abi.NewStoragePower(int64(defaultSectorSize*10)))
	feePostWg.Add(1)
	go func() {
		mminer.WaitTillPost(verifiedSector25[0])
		feePostWg.Done()
	}()

	checkAllFees := func() {
		t.Log("*** Checking daily fees on old sectors")

		for _, sn := range noFeeSectors {
			has, fee := checkDailyFee(sn)
			req.True(has) // v16
			req.Equal(abi.NewTokenAmount(0), fee)
		}

		t.Log("*** Re-checking daily fees on snapped sectors")

		has, fee = checkDailyFee(ccSectors24[0])
		req.True(has)
		req.Equal(ccSectors240ExpectedFee, fee)
		has, fee = checkDailyFee(ccSectors24[1])
		req.True(has)
		req.Equal(ccSectors241ExpectedFee, fee)

		t.Log("*** Checking daily fees on new sectors")

		has, fee = checkDailyFee(ccSectors25[0])
		req.True(has)
		req.Equal(ccSectors25ExpectedFee, fee)
		has, fee = checkDailyFee(ccSectors25[1])
		req.True(has)
		req.Equal(ccSectors25ExpectedFee, fee)
		has, fee = checkDailyFee(dealSector25[0])
		req.True(has)
		req.Equal(dealSector25ExpectedFee, fee)
		has, fee = checkDailyFee(verifiedSector25[0])
		req.True(has)
		req.Equal(verified25ExpectedFee, fee)
	}

	checkAllFees() // before PoST
	checkMiner16Invariants()

	t.Log("*** Waiting for PoST for sectors onboarded after the network upgrade")

	expectedRaw = uint64(defaultSectorSize * 8)  // 8 sectors onboarded
	expectedQap = uint64(defaultSectorSize * 35) // 5 sectors + 3 (incl 1 snap) verified sectors
	mminer.WaitTillActivatedAndAssertPower(allSectors, expectedRaw, expectedQap)

	checkAllFees() // after PoST
	checkMiner16Invariants()

	// wait for all fees to be paid—we need each one to have reached its first deadline and they are
	// likely spread out over multiple deadlines
	feePostWg.Wait()
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	// wait one deadline to make sure we get to the end of the current deadline where we've done a
	// PoST.
	// NOTE: we are crossing our fingers a little here, it is possible that our snapped sectors end
	// up hitting two proving deadlines if we didn't compress our nv25 onboarding enough and get
	// allocated to nicely aligned deadlines.
	client.WaitTillChain(ctx, kit.HeightAtLeast(head.Height()+miner.WPoStChallengeWindow()))

	var expectTotalBurn abi.TokenAmount
	for _, fee := range []abi.TokenAmount{
		ccSectors240ExpectedFee,
		ccSectors241ExpectedFee,
		ccSectors25ExpectedFee, // x2
		ccSectors25ExpectedFee,
		dealSector25ExpectedFee,
		verified25ExpectedFee,
	} {
		expectTotalBurn = big.Add(expectTotalBurn, fee)
	}
	// we've passed our first deadline where fees were payable, both for the snapped nv24 sectors
	// and the nv25 sectors
	expectMinerBurn(expectTotalBurn)
	// with only one fee payment so far for all sectors, the total in the records should be the same as the burn
	checkFeeRecords(expectTotalBurn)
	checkMiner16Invariants()
}
