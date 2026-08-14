package itests

import (
	"bytes"
	"context"
	"testing"
	"time"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/multiformats/go-multicodec"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	reward18 "github.com/filecoin-project/go-state-types/builtin/v18/reward"
	reward19 "github.com/filecoin-project/go-state-types/builtin/v19/reward"
	adt19 "github.com/filecoin-project/go-state-types/builtin/v19/util/adt"
	rewardMath "github.com/filecoin-project/go-state-types/builtin/v19/util/math"
	"github.com/filecoin-project/go-state-types/manifest"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	chainstate "github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestSolsticeRewardLifecycle(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	const (
		upgradeEpoch                = abi.ChainEpoch(30)
		activation                  = upgradeEpoch + 1
		timelock                    = abi.ChainEpoch(8)
		consensusWeightRampDuration = abi.ChainEpoch(81)
		splitSampleEpochs           = abi.ChainEpoch(10)
	)
	pct := reward19.Denom / 100

	swaKey, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)
	writerKey, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)
	recipient1Key, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)
	recipient2Key, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)

	swaID := mustIDAddress(t, 100)
	writerID := mustIDAddress(t, 101)
	recipient1ID := mustIDAddress(t, 102)
	recipient2ID := mustIDAddress(t, 103)

	bootstrapParams := buildconstants.SolsticeRewardBootstrapParams{
		SWATimelockEpochs:                 timelock,
		ConsensusWeightRampDurationEpochs: consensusWeightRampDuration,
		ConsensusWeight: buildconstants.SolsticeRewardWeightParams{
			VStart: 95 * pct,
			Floor:  50 * pct,
			Cap:    95 * pct,
		},
		ServiceWeight: buildconstants.SolsticeRewardWeightParams{
			VStart: 5 * pct,
			Floor:  5 * pct,
			Cap:    10 * pct,
		},
		SWAActor:            swaID,
		SRAActor:            writerID,
		InitialOrchestrator: recipient1ID,
	}
	rampTotal := bootstrapParams.ConsensusWeight.VStart - bootstrapParams.ConsensusWeight.Floor
	// TODO: Tie ServiceWeight.Cap-VStart to rampTotal/9 so W2_BASE cannot drift from the nine-quarter schedule.
	rampEpochs := uint64(bootstrapParams.ConsensusWeightRampDurationEpochs)
	req.NotZero(rampTotal % rampEpochs)
	bootstrapSlope := rampTotal / rampEpochs
	if rampTotal%rampEpochs != 0 {
		bootstrapSlope++
	}
	req.Positive(bootstrapSlope)
	consensusFloorEpoch := activation + bootstrapParams.ConsensusWeightRampDurationEpochs
	splitStartEpoch := consensusFloorEpoch + 1
	splitEndEpoch := splitStartEpoch + splitSampleEpochs

	client, miner, ens := kit.EnsembleMinimal(t,
		kit.MockProofs(),
		kit.ThroughRPC(),
		kit.Account(swaKey, types.FromFil(100)),
		kit.Account(writerKey, types.FromFil(100)),
		kit.Account(recipient1Key, types.FromFil(100)),
		kit.Account(recipient2Key, types.FromFil(100)),
		kit.UpgradeSchedule(
			stmgr.Upgrade{Network: network.Version28, Height: -1},
			stmgr.Upgrade{
				Network:   network.Version29,
				Height:    upgradeEpoch,
				Migration: filcns.UpgradeActorsV19With(bootstrapParams),
				PreMigrations: []stmgr.PreMigration{{
					PreMigration:    filcns.PreUpgradeActorsV19With(bootstrapParams),
					StartWithin:     15,
					DontStartWithin: 2,
					StopWithin:      2,
				}},
			},
		),
	)
	ens.InterconnectAll().BeginMining(5 * time.Millisecond)

	for _, account := range []struct {
		key      *key.Key
		expected address.Address
	}{
		{swaKey, swaID},
		{writerKey, writerID},
		{recipient1Key, recipient1ID},
		{recipient2Key, recipient2ID},
	} {
		walletAddr, err := client.WalletImport(ctx, &account.key.KeyInfo)
		req.NoError(err)
		actual, err := client.StateLookupID(ctx, walletAddr, types.EmptyTSK)
		req.NoError(err)
		req.Equal(account.expected, actual)
	}

	store := cbor.NewCborStore(blockstore.NewAPIBlockstore(client))
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+8))

	preTS := tipsetAtOrBefore(ctx, t, client, upgradeEpoch)
	req.Equal(upgradeEpoch, preTS.Height(), "upgrade epoch must contain a tipset")
	preActor, err := client.StateGetActor(ctx, builtin.RewardActorAddr, preTS.Key())
	req.NoError(err)
	var pre reward18.State
	req.NoError(store.Get(ctx, preActor.Head, &pre))
	preSimpleTotal := pre.SimpleTotal
	preBaselineTotal := pre.BaselineTotal
	// Migration must preserve the pre-funded allocation before later transitions
	// are checked against it.
	initialAllocation := big.Add(preActor.Balance, pre.TotalStoragePowerReward)

	// A tipset exposes its parent state, so the activation tipset supplies the
	// post-award v18 state consumed by the migration.
	activationTS := tipsetAtOrAfter(ctx, t, client, activation)
	migrationInputActor, err := client.StateGetActor(ctx, builtin.RewardActorAddr, activationTS.Key())
	req.NoError(err)
	var migrationInput reward18.State
	req.NoError(store.Get(ctx, migrationInputActor.Head, &migrationInput))

	// Compute from the preceding tipset to apply the activation fork without
	// applying activation-tipset messages or its block reward.
	computed, err := client.StateCompute(ctx, activation, nil, preTS.Key())
	req.NoError(err)
	migrationTree, err := chainstate.LoadStateTree(store, computed.Root)
	req.NoError(err)
	migrationActor, err := migrationTree.GetActor(builtin.RewardActorAddr)
	req.NoError(err)
	var migrated reward19.State
	req.NoError(store.Get(ctx, migrationActor.Head, &migrated))
	adtStore := adt19.WrapStore(ctx, store)
	migratedStreams, err := migrated.LoadStreams(adtStore)
	req.NoError(err)
	_, invariantMessages := reward19.CheckStateInvariants(
		&migrated, adtStore, preTS.Height(), migrationActor.Balance,
	)
	req.Empty(invariantMessages.Messages())
	req.Equal(initialAllocation, rewardAllocationAt(t, migrationActor, &migrated, migratedStreams))

	lifecycle := &solsticeRewardLifecycle{
		ctx:                 ctx,
		client:              client,
		miner:               miner,
		store:               store,
		pct:                 pct,
		upgradeEpoch:        upgradeEpoch,
		activation:          activation,
		timelock:            timelock,
		bootstrapSlope:      bootstrapSlope,
		splitStart:          splitStartEpoch,
		splitEnd:            splitEndEpoch,
		bootstrap:           bootstrapParams,
		swaKey:              swaKey,
		writerKey:           writerKey,
		swaID:               swaID,
		writerID:            writerID,
		recipients:          []address.Address{recipient1ID, recipient2ID},
		preTS:               preTS,
		activationTS:        activationTS,
		preSimpleTotal:      preSimpleTotal,
		preBaselineTotal:    preBaselineTotal,
		initialAllocation:   initialAllocation,
		migrationInputActor: migrationInputActor,
		migrationActor:      migrationActor,
		migrationInput:      migrationInput,
		migrated:            migrated,
		migratedStreams:     migratedStreams,
	}

	t.Run("migration and award continuity", lifecycle.testMigrationAndAwardContinuity)
	t.Run("circulating supply continuity", lifecycle.testCirculatingSupplyContinuity)
	t.Run("reward total constants replace stored state", lifecycle.testRewardTotalConstants)
	t.Run("sloped bootstrap weight", lifecycle.testSlopedBootstrapWeight)
	t.Run("reward split economics", lifecycle.testRewardSplitEconomics)
	t.Run("share settlement and wallet payouts", lifecycle.testShareSettlementAndWalletPayouts)
	t.Run("queue controls and write event", lifecycle.testQueueControlsAndEvent)
	t.Run("deferred weight schedule", lifecycle.testDeferredWeightSchedule)
	t.Run("remove stream tombstone claim", lifecycle.testRemoveStreamTombstoneClaim)
}

func mustIDAddress(t *testing.T, id uint64) address.Address {
	t.Helper()
	req := require.New(t)
	addr, err := address.NewIDAddress(id)
	req.NoError(err)
	return addr
}

// tipsetAtOrBefore names ChainGetTipSetByHeight's null-round fallback.
func tipsetAtOrBefore(ctx context.Context, t *testing.T, node api.FullNode, target abi.ChainEpoch) *types.TipSet {
	t.Helper()
	req := require.New(t)
	ts, err := node.ChainGetTipSetByHeight(ctx, target, types.EmptyTSK)
	req.NoError(err)
	req.LessOrEqual(ts.Height(), target)
	return ts
}

// tipsetAtOrAfter skips null epochs and returns the first tipset at or after target.
func tipsetAtOrAfter(ctx context.Context, t *testing.T, node api.FullNode, target abi.ChainEpoch) *types.TipSet {
	t.Helper()
	req := require.New(t)
	head, err := node.ChainHead(ctx)
	req.NoError(err)
	req.GreaterOrEqual(head.Height(), target, "chain head has not reached target epoch")
	for height := target; height <= head.Height(); height++ {
		ts, err := node.ChainGetTipSetByHeight(ctx, height, head.Key())
		req.NoError(err)
		if ts.Height() >= target {
			return ts
		}
	}
	req.FailNow("no non-null tipset after target", "target epoch %d through head %d", target, head.Height())
	return nil
}

func mustActor(ctx context.Context, t *testing.T, node api.FullNode, addr address.Address, tsk types.TipSetKey) *types.Actor {
	t.Helper()
	req := require.New(t)
	actor, err := node.StateGetActor(ctx, addr, tsk)
	req.NoError(err)
	return actor
}

// loadReward19 reads inline and offboarded state together and checks invariants
// against the chain epoch represented by the observed tipset.
func loadReward19(ctx context.Context, t *testing.T, node api.FullNode, store cbor.IpldStore, tsk types.TipSetKey) (*types.Actor, *reward19.State, *reward19.StreamsState) {
	t.Helper()
	req := require.New(t)
	tipset, err := node.ChainGetTipSet(ctx, tsk)
	req.NoError(err)
	parent, err := node.ChainGetTipSet(ctx, tipset.Parents())
	req.NoError(err)
	actor := mustActor(ctx, t, node, builtin.RewardActorAddr, tsk)
	var state reward19.State
	req.NoError(store.Get(ctx, actor.Head, &state))
	adtStore := adt19.WrapStore(ctx, store)
	streams, err := state.LoadStreams(adtStore)
	req.NoError(err)
	_, messages := reward19.CheckStateInvariants(&state, adtStore, parent.Height(), actor.Balance)
	req.Empty(messages.Messages())
	return actor, &state, streams
}

// adjacentReward19States finds a single-epoch transition so reward recomputation does not span null rounds.
func adjacentReward19States(
	ctx context.Context,
	t *testing.T,
	node api.FullNode,
	store cbor.IpldStore,
	start abi.ChainEpoch,
) (*reward19.State, *reward19.State) {
	t.Helper()
	req := require.New(t)
	head, err := node.ChainHead(ctx)
	req.NoError(err)
	req.GreaterOrEqual(head.Height(), start, "chain head has not reached search start")
	for height := start; height <= head.Height(); height++ {
		ts, err := node.ChainGetTipSetByHeight(ctx, height, head.Key())
		req.NoError(err)
		parent, err := node.ChainGetTipSet(ctx, ts.Parents())
		req.NoError(err)
		if ts.Height() != parent.Height()+1 {
			continue
		}
		_, previous, _ := loadReward19(ctx, t, node, store, parent.Key())
		_, current, _ := loadReward19(ctx, t, node, store, ts.Key())
		if current.Epoch == previous.Epoch+1 {
			return previous, current
		}
	}
	req.FailNow("no adjacent v19 reward states", "epoch %d through head %d", start, head.Height())
	return nil, nil
}

// computeRewardWithTotals mirrors the v19 calculation with caller-supplied issuance totals.
// The lifecycle uses it to distinguish code constants from the v18 fields removed by migration.
func computeRewardWithTotals(
	epoch abi.ChainEpoch,
	prevTheta big.Int,
	currTheta big.Int,
	simpleTotal abi.TokenAmount,
	baselineTotal abi.TokenAmount,
) abi.TokenAmount {
	simpleReward := big.Mul(simpleTotal, reward19.ExpLamSubOne)
	epochLam := big.Mul(big.NewInt(int64(epoch)), reward19.Lambda)
	simpleReward = big.Mul(simpleReward, big.NewFromGo(rewardMath.ExpNeg(epochLam.Int)))
	simpleReward = big.Rsh(simpleReward, rewardMath.Precision128)

	baselineReward := big.Sub(
		computeBaselineSupplyWithTotal(currTheta, baselineTotal),
		computeBaselineSupplyWithTotal(prevTheta, baselineTotal),
	)
	return big.Rsh(big.Add(simpleReward, baselineReward), rewardMath.Precision128)
}

// computeBaselineSupplyWithTotal is ComputeBaselineSupply with an injectable allocation total.
func computeBaselineSupplyWithTotal(theta big.Int, baselineTotal abi.TokenAmount) abi.TokenAmount {
	thetaLam := big.Mul(theta, reward19.Lambda)
	thetaLam = big.Rsh(thetaLam, rewardMath.Precision128)
	expNegThetaLam := big.NewFromGo(rewardMath.ExpNeg(thetaLam.Int))
	one := big.Lsh(big.NewInt(1), rewardMath.Precision128)
	return big.Mul(baselineTotal, big.Sub(one, expNegThetaLam))
}

// explicitServiceLiabilities totals service rewards that f02 still holds for claims.
// A live explicit stream owes its current accrual, less same-period claims, plus
// payables carried from closed periods. A tombstone owes only its payables.
func explicitServiceLiabilities(t *testing.T, state *reward19.State, streams *reward19.StreamsState) abi.TokenAmount {
	t.Helper()
	req := require.New(t)
	total := big.Zero()
	accruals := make(map[reward19.StreamID]abi.TokenAmount, len(state.Accrued))
	for _, accrual := range state.Accrued {
		accruals[accrual.ID] = accrual.Amount
	}
	for _, stream := range streams.Streams {
		if stream.Distribution == nil {
			continue
		}
		amount, ok := accruals[stream.ID]
		req.True(ok)
		total = big.Add(total, amount)
		for _, row := range stream.Distribution.Payable {
			total = big.Add(total, row.Amount)
		}
		for _, row := range stream.Distribution.ClaimedPeriod {
			total = big.Sub(total, row.Amount)
		}
	}
	for _, tombstone := range streams.Tombstones {
		for _, row := range tombstone.Payable {
			total = big.Add(total, row.Amount)
		}
	}
	return total
}

// rewardAllocationAt reconstructs the lifetime reward allocation implied by an f02 snapshot.
//
// At rest, f02's balance is the unissued reserve plus explicit rewards retained
// for claims. TotalMintedReward is cumulative gross issuance from the reserve:
//
//	allocation = TotalMintedReward + actor balance - explicit service liabilities
//
// Awards preserve the result by dividing gross issuance among miner payment,
// burn, and service liability. Claims reduce balance and liability equally.
func rewardAllocationAt(t *testing.T, actor *types.Actor, state *reward19.State, streams *reward19.StreamsState) abi.TokenAmount {
	t.Helper()
	explicitLiabilities := explicitServiceLiabilities(t, state, streams)
	remainingReserve := big.Sub(actor.Balance, explicitLiabilities)
	return big.Add(state.TotalMintedReward, remainingReserve)
}

// requireShareWithinAtto compares fixed-point cross-products with a rounding
// allowance expressed in attoFIL, not percentage points.
func requireShareWithinAtto(t *testing.T, part abi.TokenAmount, total abi.TokenAmount, share uint64, roundingAtto int64) {
	t.Helper()
	req := require.New(t)
	req.GreaterOrEqual(roundingAtto, int64(0))
	scaledActual := big.Mul(part, big.NewInt(int64(reward19.Denom)))
	scaledExpected := big.Mul(total, big.NewInt(int64(share)))
	scaledError := big.Sub(scaledActual, scaledExpected).Abs()
	scaledSlack := big.Mul(big.NewInt(roundingAtto), big.NewInt(int64(reward19.Denom)))
	req.LessOrEqual(big.Cmp(scaledError, scaledSlack), 0)
}

type solsticeRewardLifecycle struct {
	ctx    context.Context
	client *kit.TestFullNode
	miner  *kit.TestMiner
	store  cbor.IpldStore

	pct            uint64
	upgradeEpoch   abi.ChainEpoch
	activation     abi.ChainEpoch
	timelock       abi.ChainEpoch
	bootstrapSlope uint64
	splitStart     abi.ChainEpoch
	splitEnd       abi.ChainEpoch
	bootstrap      buildconstants.SolsticeRewardBootstrapParams

	swaKey     *key.Key
	writerKey  *key.Key
	swaID      address.Address
	writerID   address.Address
	recipients []address.Address

	preTS               *types.TipSet
	activationTS        *types.TipSet
	preSimpleTotal      abi.TokenAmount
	preBaselineTotal    abi.TokenAmount
	initialAllocation   abi.TokenAmount
	migrationInputActor *types.Actor
	migrationActor      *types.Actor
	migrationInput      reward18.State
	migrated            reward19.State
	migratedStreams     *reward19.StreamsState
}

type solsticeRewardDeltas struct {
	total   abi.TokenAmount
	service abi.TokenAmount
	burn    abi.TokenAmount
	miner   abi.TokenAmount
}

type solsticeClaimResult struct {
	tipset  *types.TipSet
	amounts []abi.TokenAmount
	deltas  []abi.TokenAmount
	state   *reward19.State
	streams *reward19.StreamsState
}

// testMigrationAndAwardContinuity verifies the exact v18-to-v19 state cutover
// and confirms that awards continue from the migrated state.
func (f *solsticeRewardLifecycle) testMigrationAndAwardContinuity(t *testing.T) {
	req := require.New(t)
	expectedCode, ok := actors.GetActorCodeID(actorstypes.Version19, manifest.RewardKey)
	req.True(ok)
	req.Equal(expectedCode, f.migrationActor.Code)

	req.Equal(f.migrationInputActor.Balance, f.migrationActor.Balance)
	req.Equal(f.migrationInput.CumsumBaseline, f.migrated.CumsumBaseline)
	req.Equal(f.migrationInput.CumsumRealized, f.migrated.CumsumRealized)
	req.Equal(f.migrationInput.EffectiveNetworkTime, f.migrated.EffectiveNetworkTime)
	req.Equal(f.migrationInput.EffectiveBaselinePower, f.migrated.EffectiveBaselinePower)
	req.Equal(f.migrationInput.ThisEpochReward, f.migrated.ThisEpochReward)
	req.Equal(f.migrationInput.ThisEpochRewardSmoothed.PositionEstimate, f.migrated.ThisEpochRewardSmoothed.PositionEstimate)
	req.Equal(f.migrationInput.ThisEpochRewardSmoothed.VelocityEstimate, f.migrated.ThisEpochRewardSmoothed.VelocityEstimate)
	req.Equal(f.migrationInput.ThisEpochBaselinePower, f.migrated.ThisEpochBaselinePower)
	req.Equal(f.migrationInput.Epoch, f.migrated.Epoch)
	req.Equal(f.migrationInput.TotalStoragePowerReward, f.migrated.TotalMintedReward)
	req.Equal(big.Zero(), f.migrated.TotalBurnMinted)
	req.Equal(big.Zero(), f.migrated.TotalExplicitMinted)
	req.Equal([]reward19.StreamAccrual{{ID: 2, Amount: big.Zero()}}, f.migrated.Accrued)
	req.Equal(f.timelock, f.migrated.SWATimelockEpochs)
	req.Equal(f.swaID, f.migrated.SWAActor)
	req.Len(f.migratedStreams.Streams, 2)
	req.Empty(f.migratedStreams.PendingWrites)
	req.Empty(f.migratedStreams.Tombstones)
	req.Equal(f.activation, f.migratedStreams.Streams[0].Weight.TStart)
	req.Equal(f.activation, f.migratedStreams.Streams[1].Weight.TStart)
	req.Equal(f.bootstrap.ConsensusWeight.VStart, f.migratedStreams.Streams[0].Weight.VStart)
	req.Equal(f.bootstrap.ServiceWeight.VStart, f.migratedStreams.Streams[1].Weight.VStart)
	req.Equal(-int64(f.bootstrapSlope), f.migratedStreams.Streams[0].Weight.Slope)
	req.Equal(int64(f.bootstrapSlope), f.migratedStreams.Streams[1].Weight.Slope)
	req.Equal(f.writerID, f.migratedStreams.Streams[1].Distribution.Writer)
	req.Equal([]reward19.RecipientShare{{Recipient: f.recipients[0], Share: reward19.Denom}}, f.migratedStreams.Streams[1].Distribution.Shares)

	laterTS := tipsetAtOrAfter(f.ctx, t, f.client, f.upgradeEpoch+8)
	laterActor, later, laterStreams := loadReward19(f.ctx, t, f.client, f.store, laterTS.Key())
	req.Positive(big.Cmp(later.TotalMintedReward, f.migrated.TotalMintedReward))
	req.Positive(big.Cmp(later.TotalExplicitMinted, f.migrated.TotalExplicitMinted))
	f.requireAllocation(t, laterActor, later, laterStreams)

	preMiner := mustActor(f.ctx, t, f.client, f.miner.ActorAddr, f.preTS.Key())
	postMiner := mustActor(f.ctx, t, f.client, f.miner.ActorAddr, laterTS.Key())
	req.Positive(big.Cmp(postMiner.Balance, preMiner.Balance))
}

// testCirculatingSupplyContinuity proves that Lotus derives FilMined from the
// renamed cumulative minted total across and after the migration.
func (f *solsticeRewardLifecycle) testCirculatingSupplyContinuity(t *testing.T) {
	req := require.New(t)
	postMigrationTS := tipsetAtOrAfter(f.ctx, t, f.client, f.activationTS.Height()+1)
	_, postMigration, _ := loadReward19(f.ctx, t, f.client, f.store, postMigrationTS.Key())
	preSupply, err := f.client.StateVMCirculatingSupplyInternal(f.ctx, f.activationTS.Key())
	req.NoError(err)
	postSupply, err := f.client.StateVMCirculatingSupplyInternal(f.ctx, postMigrationTS.Key())
	req.NoError(err)
	req.Equal(
		big.Sub(postMigration.TotalMintedReward, f.migrationInput.TotalStoragePowerReward),
		big.Sub(postSupply.FilMined, preSupply.FilMined),
	)

	laterEpoch := postMigrationTS.Height() + 7
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(laterEpoch+1))
	laterTS := tipsetAtOrAfter(f.ctx, t, f.client, laterEpoch)
	_, later, _ := loadReward19(f.ctx, t, f.client, f.store, laterTS.Key())
	laterSupply, err := f.client.StateVMCirculatingSupplyInternal(f.ctx, laterTS.Key())
	req.NoError(err)
	req.Equal(
		big.Sub(later.TotalMintedReward, postMigration.TotalMintedReward),
		big.Sub(laterSupply.FilMined, postSupply.FilMined),
	)
}

// testRewardTotalConstants proves reward calculation uses the v19 issuance
// constants rather than the retired totals stored in v18 state.
func (f *solsticeRewardLifecycle) testRewardTotalConstants(t *testing.T) {
	req := require.New(t)
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(f.activation+25))
	previous, current := adjacentReward19States(f.ctx, t, f.client, f.store, f.activation+5)
	prevTheta := reward19.ComputeRTheta(
		previous.EffectiveNetworkTime,
		previous.EffectiveBaselinePower,
		previous.CumsumRealized,
		previous.CumsumBaseline,
	)
	currTheta := reward19.ComputeRTheta(
		current.EffectiveNetworkTime,
		current.EffectiveBaselinePower,
		current.CumsumRealized,
		current.CumsumBaseline,
	)
	fromConstants := computeRewardWithTotals(
		current.Epoch, prevTheta, currTheta, reward19.SimpleTotal, reward19.BaselineTotal,
	)
	fromStoredTotals := computeRewardWithTotals(
		current.Epoch, prevTheta, currTheta, f.preSimpleTotal, f.preBaselineTotal,
	)

	req.NotEqual(fromStoredTotals, fromConstants)
	req.Equal(fromConstants, current.ThisEpochReward)
	req.NotEqual(fromStoredTotals, current.ThisEpochReward)
}

// testSlopedBootstrapWeight observes the consensus ramp reducing the miner
// share by a lower bound derived from its on-chain slope.
func (f *solsticeRewardLifecycle) testSlopedBootstrapWeight(t *testing.T) {
	req := require.New(t)
	const window = abi.ChainEpoch(5)
	earlyStartEpoch := f.activation + 2
	earlyEndEpoch := earlyStartEpoch + window
	serviceRise := f.bootstrap.ServiceWeight.Cap - f.bootstrap.ServiceWeight.VStart
	serviceClampOffset := (serviceRise + f.bootstrapSlope - 1) / f.bootstrapSlope
	req.Less(uint64(earlyEndEpoch-f.activation), serviceClampOffset)
	lateStartEpoch := f.activation + 30
	lateEndEpoch := lateStartEpoch + window
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(lateEndEpoch+1))

	_, earlyStart, _ := f.stateAtOrAfter(t, earlyStartEpoch)
	_, earlyEnd, _ := f.stateAtOrAfter(t, earlyEndEpoch)
	_, lateStart, _ := f.stateAtOrAfter(t, lateStartEpoch)
	_, lateEnd, _ := f.stateAtOrAfter(t, lateEndEpoch)
	early := rewardDeltas(earlyStart, earlyEnd)
	late := rewardDeltas(lateStart, lateEnd)
	req.Positive(early.total.Sign())
	req.Positive(late.total.Sign())
	earlyAwardCount := int64(earlyEnd.Epoch - earlyStart.Epoch)
	req.Positive(earlyAwardCount)
	req.LessOrEqual(big.Cmp(early.burn, big.NewInt(earlyAwardCount)), 0, "bootstrap burn %s exceeds %d-award rounding bound", early.burn, earlyAwardCount)

	separation := lateStart.Epoch - earlyEnd.Epoch - 1
	req.Positive(separation)
	minimumDrop := uint64(separation) * f.bootstrapSlope
	actualDrop := big.Sub(big.Mul(early.miner, late.total), big.Mul(late.miner, early.total))
	minimumDropValue := big.Div(
		big.Mul(big.Mul(early.total, late.total), big.NewInt(int64(minimumDrop))),
		big.NewInt(int64(reward19.Denom)),
	)
	earlyRoundingSlack := big.Mul(big.NewInt(int64(earlyEnd.Epoch-earlyStart.Epoch)), late.total)
	req.GreaterOrEqual(big.Cmp(actualDrop, big.Sub(minimumDropValue, earlyRoundingSlack)), 0)
}

// testRewardSplitEconomics checks the settled bootstrap weights divide gross
// issuance exactly among miner, service, and burn.
func (f *solsticeRewardLifecycle) testRewardSplitEconomics(t *testing.T) {
	req := require.New(t)
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(f.splitEnd+1))
	startActor, start, startStreams := f.stateAtOrAfter(t, f.splitStart)
	endActor, end, endStreams := f.stateAtOrAfter(t, f.splitEnd)
	delta := rewardDeltas(start, end)
	req.Positive(delta.total.Sign())
	req.Positive(delta.service.Sign())
	req.Positive(delta.burn.Sign())
	req.Positive(delta.miner.Sign())
	awardUpperBound := int64(end.Epoch - start.Epoch)
	req.Positive(awardUpperBound)
	expectedBurnShare := reward19.Denom - f.bootstrap.ConsensusWeight.Floor - f.bootstrap.ServiceWeight.Cap
	requireShareWithinAtto(t, delta.service, delta.total, f.bootstrap.ServiceWeight.Cap, awardUpperBound)
	requireShareWithinAtto(t, delta.miner, delta.total, f.bootstrap.ConsensusWeight.Floor, awardUpperBound)
	requireShareWithinAtto(t, delta.burn, delta.total, expectedBurnShare, 2*awardUpperBound)
	f.requireAllocation(t, startActor, start, startStreams)
	f.requireAllocation(t, endActor, end, endStreams)
}

// testShareSettlementAndWalletPayouts exercises share replacement, partial
// Claims, carried entitlements, and exact recipient balance changes.
func (f *solsticeRewardLifecycle) testShareSettlementAndWalletPayouts(t *testing.T) {
	req := require.New(t)
	lookup := f.sendRewardMessage(t, f.writerKey.Address, builtin.MethodsReward.SetSharesExported, &reward19.SetSharesParams{
		ID: 2,
		Shares: []reward19.RecipientShare{
			{Recipient: f.recipients[0], Share: 40 * f.pct},
			{Recipient: f.recipients[1], Share: 60 * f.pct},
		},
	})
	requireMessageSuccess(t, lookup)

	settledActor, settledState, settledStreams := loadReward19(f.ctx, t, f.client, f.store, lookup.TipSet)
	f.requireAllocation(t, settledActor, settledState, settledStreams)
	distribution := settledStreams.Streams[1].Distribution
	req.NotNil(distribution)
	req.Equal([]reward19.RecipientShare{
		{Recipient: f.recipients[0], Share: 40 * f.pct},
		{Recipient: f.recipients[1], Share: 60 * f.pct},
	}, distribution.Shares)
	req.Len(distribution.Payable, 1)
	req.Equal(f.recipients[0], distribution.Payable[0].Recipient)
	settledPayable := distribution.Payable[0].Amount

	setSharesTS, err := f.client.ChainGetTipSet(f.ctx, lookup.TipSet)
	req.NoError(err)
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(setSharesTS.Height()+5))
	beforeClaimsTS := tipsetAtOrAfter(f.ctx, t, f.client, setSharesTS.Height()+5)
	beforeBalances := []abi.TokenAmount{
		mustActor(f.ctx, t, f.client, f.recipients[0], beforeClaimsTS.Key()).Balance,
		mustActor(f.ctx, t, f.client, f.recipients[1], beforeClaimsTS.Key()).Balance,
	}

	first := f.claim(t, f.recipients[:1])
	firstDistribution := first.streams.Streams[1].Distribution
	req.Empty(firstDistribution.Payable)
	req.Len(firstDistribution.ClaimedPeriod, 1)
	req.Equal(f.recipients[0], firstDistribution.ClaimedPeriod[0].Recipient)
	firstCurrentClaim := firstDistribution.ClaimedPeriod[0].Amount
	req.Positive(firstCurrentClaim.Sign())
	req.Equal(big.Add(settledPayable, firstCurrentClaim), first.amounts[0])
	req.Equal(reward19.StreamID(2), first.state.Accrued[0].ID)
	pendingRecipient2 := accruedShare(first.state.Accrued[0].Amount, 60*f.pct)
	req.Positive(pendingRecipient2.Sign())

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(first.tipset.Height()+5))
	second := f.claim(t, f.recipients)
	secondDistribution := second.streams.Streams[1].Distribution
	req.Empty(secondDistribution.Payable)
	req.Len(secondDistribution.ClaimedPeriod, 2)
	req.Equal(f.recipients[0], secondDistribution.ClaimedPeriod[0].Recipient)
	req.Equal(f.recipients[1], secondDistribution.ClaimedPeriod[1].Recipient)
	req.Equal(reward19.StreamID(2), second.state.Accrued[0].ID)

	currentRecipient1 := accruedShare(second.state.Accrued[0].Amount, 40*f.pct)
	currentRecipient2 := accruedShare(second.state.Accrued[0].Amount, 60*f.pct)
	req.Positive(big.Cmp(currentRecipient1, secondDistribution.ClaimedPeriod[0].Amount))
	req.Positive(big.Cmp(currentRecipient2, secondDistribution.ClaimedPeriod[1].Amount))
	req.Equal(big.Sub(secondDistribution.ClaimedPeriod[0].Amount, firstCurrentClaim), second.amounts[0])
	req.Equal(secondDistribution.ClaimedPeriod[1].Amount, second.amounts[1])
	req.Positive(big.Cmp(second.amounts[1], pendingRecipient2))

	cumulativePaid := []abi.TokenAmount{big.Add(first.amounts[0], second.amounts[0]), second.amounts[1]}
	req.Equal(big.Add(settledPayable, secondDistribution.ClaimedPeriod[0].Amount), cumulativePaid[0])
	req.Equal(secondDistribution.ClaimedPeriod[1].Amount, cumulativePaid[1])
	for i, recipient := range f.recipients {
		after := mustActor(f.ctx, t, f.client, recipient, second.tipset.Key()).Balance
		req.Equal(cumulativePaid[i], big.Sub(after, beforeBalances[i]))
	}

	currentPeriodTotal := big.Add(secondDistribution.ClaimedPeriod[0].Amount, secondDistribution.ClaimedPeriod[1].Amount)
	requireShareWithinAtto(t, secondDistribution.ClaimedPeriod[0].Amount, currentPeriodTotal, 40*f.pct, 1)
	requireShareWithinAtto(t, secondDistribution.ClaimedPeriod[1].Amount, currentPeriodTotal, 60*f.pct, 1)
}

// testQueueControlsAndEvent checks SWA authorization, occupied-slot rejection,
// cancellation, and the chain-visible write-queued payload.
func (f *solsticeRewardLifecycle) testQueueControlsAndEvent(t *testing.T) {
	req := require.New(t)
	params := &reward19.SetWeightRecordsParams{Updates: []reward19.WeightRecordUpdate{
		{ID: 1, Weight: flatWeight(65 * f.pct)},
		{ID: 2, Weight: flatWeight(10 * f.pct)},
	}}
	beforeTS, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	_, _, beforeStreams := loadReward19(f.ctx, t, f.client, f.store, beforeTS.Key())
	beforeWeights := []reward19.WeightRecord{beforeStreams.Streams[0].Weight, beforeStreams.Streams[1].Weight}

	unauthorized := f.sendRewardMessage(t, f.writerKey.Address, builtin.MethodsReward.SetWeightRecordsExported, params)
	req.False(unauthorized.Receipt.ExitCode.IsSuccess())

	queuedLookup := f.sendRewardMessage(t, f.swaKey.Address, builtin.MethodsReward.SetWeightRecordsExported, params)
	requireMessageSuccess(t, queuedLookup)
	queuedActor, queued, queuedStreams := loadReward19(f.ctx, t, f.client, f.store, queuedLookup.TipSet)
	f.requireAllocation(t, queuedActor, queued, queuedStreams)
	req.Len(queuedStreams.PendingWrites, 1)
	pending := queuedStreams.PendingWrites[0]
	req.Equal(reward19.PendingWriteOpSetWeightRecords, pending.Op)

	collision := f.sendRewardMessage(t, f.swaKey.Address, builtin.MethodsReward.SetWeightRecordsExported, params)
	req.False(collision.Receipt.ExitCode.IsSuccess())
	cancel := f.sendRewardMessage(t, f.swaKey.Address, builtin.MethodsReward.CancelPendingExported, &reward19.CancelPendingParams{
		Op: reward19.PendingWriteOpSetWeightRecords,
	})
	requireMessageSuccess(t, cancel)
	cancelActor, cancelled, cancelledStreams := loadReward19(f.ctx, t, f.client, f.store, cancel.TipSet)
	f.requireAllocation(t, cancelActor, cancelled, cancelledStreams)
	req.Empty(cancelledStreams.PendingWrites)
	req.Equal(beforeWeights[0], cancelledStreams.Streams[0].Weight)
	req.Equal(beforeWeights[1], cancelledStreams.Streams[1].Weight)
	requireWriteQueuedEvent(f.ctx, t, f.client, pending)

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(pending.EffectiveEpoch+2))
	_, _, afterStreams := f.stateAtOrAfter(t, pending.EffectiveEpoch+1)
	req.Empty(afterStreams.PendingWrites)
	req.Equal(beforeWeights[0], afterStreams.Streams[0].Weight)
	req.Equal(beforeWeights[1], afterStreams.Streams[1].Weight)
}

// testDeferredWeightSchedule proves a queued schedule remains inert through
// its hold, applies when due, and controls subsequent reward allocation.
func (f *solsticeRewardLifecycle) testDeferredWeightSchedule(t *testing.T) {
	req := require.New(t)
	consensusWeight := flatWeight(70 * f.pct)
	serviceWeight := flatWeight(10 * f.pct)
	lookup := f.sendRewardMessage(t, f.swaKey.Address, builtin.MethodsReward.SetWeightRecordsExported, &reward19.SetWeightRecordsParams{
		Updates: []reward19.WeightRecordUpdate{{ID: 1, Weight: consensusWeight}, {ID: 2, Weight: serviceWeight}},
	})
	requireMessageSuccess(t, lookup)

	receiptTS, err := f.client.ChainGetTipSet(f.ctx, lookup.TipSet)
	req.NoError(err)
	parentTS, err := f.client.ChainGetTipSet(f.ctx, receiptTS.Parents())
	req.NoError(err)
	parentActor, parentState, parentStreams := loadReward19(f.ctx, t, f.client, f.store, parentTS.Key())
	queuedActor, queuedState, queuedStreams := loadReward19(f.ctx, t, f.client, f.store, receiptTS.Key())
	f.requireAllocation(t, parentActor, parentState, parentStreams)
	f.requireAllocation(t, queuedActor, queuedState, queuedStreams)
	req.Equal(parentStreams.Streams[0].Weight, queuedStreams.Streams[0].Weight)
	req.Equal(parentStreams.Streams[1].Weight, queuedStreams.Streams[1].Weight)
	req.Len(queuedStreams.PendingWrites, 1)
	pending := queuedStreams.PendingWrites[0]
	req.Nil(pending.ID)
	req.Equal(reward19.PendingWriteOpSetWeightRecords, pending.Op)
	req.Equal(parentTS.Height()+f.timelock, pending.EffectiveEpoch)

	// TODO: Exercise EffectiveEpoch as a null round; this scenario currently covers only a due write on a non-null epoch.
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(pending.EffectiveEpoch+10))
	dueAwardTS := tipsetAtOrAfter(f.ctx, t, f.client, pending.EffectiveEpoch)
	dueActor, dueState, dueStreams := loadReward19(f.ctx, t, f.client, f.store, dueAwardTS.Key())
	f.requireAllocation(t, dueActor, dueState, dueStreams)
	req.Len(dueStreams.PendingWrites, 1)
	req.Equal(parentStreams.Streams[0].Weight, dueStreams.Streams[0].Weight)
	req.Equal(parentStreams.Streams[1].Weight, dueStreams.Streams[1].Weight)

	appliedTS := tipsetAtOrAfter(f.ctx, t, f.client, dueAwardTS.Height()+1)
	appliedActor, appliedState, appliedStreams := loadReward19(f.ctx, t, f.client, f.store, appliedTS.Key())
	f.requireAllocation(t, appliedActor, appliedState, appliedStreams)
	req.Empty(appliedStreams.PendingWrites)
	req.Equal(consensusWeight, appliedStreams.Streams[0].Weight)
	req.Equal(serviceWeight, appliedStreams.Streams[1].Weight)

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(appliedTS.Height()+20))
	endActor, endState, endStreams := f.stateAtOrAfter(t, appliedTS.Height()+10)
	f.requireAllocation(t, endActor, endState, endStreams)
	req.Equal(consensusWeight, endStreams.Streams[0].Weight)
	req.Equal(serviceWeight, endStreams.Streams[1].Weight)
	delta := rewardDeltas(appliedState, endState)
	awardUpperBound := int64(endState.Epoch - appliedState.Epoch)
	req.Positive(awardUpperBound)
	requireShareWithinAtto(t, delta.service, delta.total, 10*f.pct, awardUpperBound)
	requireShareWithinAtto(t, delta.burn, delta.total, 20*f.pct, 2*awardUpperBound)
	requireShareWithinAtto(t, delta.miner, delta.total, 70*f.pct, awardUpperBound)
}

// testRemoveStreamTombstoneClaim follows an explicit stream from queued removal
// through tombstoning, payout, and final tombstone deletion.
func (f *solsticeRewardLifecycle) testRemoveStreamTombstoneClaim(t *testing.T) {
	req := require.New(t)
	setShares := f.sendRewardMessage(t, f.writerKey.Address, builtin.MethodsReward.SetSharesExported, &reward19.SetSharesParams{
		ID: 2,
		Shares: []reward19.RecipientShare{
			{Recipient: f.recipients[0], Share: 40 * f.pct},
			{Recipient: f.recipients[1], Share: 60 * f.pct},
		},
	})
	requireMessageSuccess(t, setShares)

	lookup := f.sendRewardMessage(t, f.swaKey.Address, builtin.MethodsReward.RemoveStreamExported, &reward19.RemoveStreamParams{ID: 2})
	requireMessageSuccess(t, lookup)
	_, _, queuedStreams := loadReward19(f.ctx, t, f.client, f.store, lookup.TipSet)
	req.Len(queuedStreams.PendingWrites, 1)
	pending := queuedStreams.PendingWrites[0]
	req.NotNil(pending.ID)
	req.Equal(reward19.StreamID(2), *pending.ID)
	req.Equal(reward19.PendingWriteOpRemoveStream, pending.Op)

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(pending.EffectiveEpoch+2))
	dueTS := tipsetAtOrAfter(f.ctx, t, f.client, pending.EffectiveEpoch)
	appliedTS := tipsetAtOrAfter(f.ctx, t, f.client, dueTS.Height()+1)
	removedActor, removed, removedStreams := loadReward19(f.ctx, t, f.client, f.store, appliedTS.Key())
	f.requireAllocation(t, removedActor, removed, removedStreams)
	req.Len(removedStreams.Streams, 1)
	req.Equal(reward19.StreamID(1), removedStreams.Streams[0].ID)
	req.Len(removedStreams.Tombstones, 1)
	req.Equal(reward19.StreamID(2), removedStreams.Tombstones[0].ID)
	req.Len(removedStreams.Tombstones[0].Payable, 2)
	payable := append([]reward19.RecipientAmount(nil), removedStreams.Tombstones[0].Payable...)
	// TODO: Sample post-removal awards and assert stream 2's former weight is burned.

	claim := f.claim(t, f.recipients)
	req.Len(claim.amounts, len(payable))
	for i, row := range payable {
		req.Equal(f.recipients[i], row.Recipient)
		req.Equal(row.Amount, claim.amounts[i])
		req.Equal(row.Amount, claim.deltas[i])
		req.Positive(claim.amounts[i].Sign())
	}
	req.Empty(claim.streams.Tombstones)
	req.Len(claim.streams.Streams, 1)
	// TODO: Re-claim the drained tombstone and assert every returned amount is zero.
}

func (f *solsticeRewardLifecycle) sendRewardMessage(t *testing.T, from address.Address, method abi.MethodNum, params cbg.CBORMarshaler) *api.MsgLookup {
	t.Helper()
	req := require.New(t)
	serialized, serializeErr := actors.SerializeParams(params)
	req.NoError(serializeErr)
	nonce, err := f.client.MpoolGetNonce(f.ctx, from)
	req.NoError(err)
	message := &types.Message{
		To: builtin.RewardActorAddr, From: from, Nonce: nonce, Value: big.Zero(),
		Method: method, Params: serialized, GasLimit: 10_000_000,
		GasFeeCap: abi.NewTokenAmount(10_000), GasPremium: big.Zero(),
	}
	signed, err := f.client.WalletSignMessage(f.ctx, from, message)
	req.NoError(err)
	messageCID, err := f.client.MpoolPush(f.ctx, signed)
	req.NoError(err)
	lookup, err := f.client.StateWaitMsg(f.ctx, messageCID, 1, api.LookbackNoLimit, true)
	req.NoError(err)
	return lookup
}

func (f *solsticeRewardLifecycle) claim(t *testing.T, wallets []address.Address) solsticeClaimResult {
	t.Helper()
	req := require.New(t)
	lookup := f.sendRewardMessage(t, f.miner.OwnerKey.Address, builtin.MethodsReward.ClaimExported, &reward19.ClaimParams{ID: 2, Wallets: wallets})
	requireMessageSuccess(t, lookup)
	var result reward19.ClaimReturn
	req.NoError(result.UnmarshalCBOR(bytes.NewReader(lookup.Receipt.Return)))
	req.Len(result.Amounts, len(wallets))

	claimTS, err := f.client.ChainGetTipSet(f.ctx, lookup.TipSet)
	req.NoError(err)
	beforeTS, err := f.client.ChainGetTipSet(f.ctx, claimTS.Parents())
	req.NoError(err)
	deltas := make([]abi.TokenAmount, len(wallets))
	for i, recipient := range wallets {
		before := mustActor(f.ctx, t, f.client, recipient, beforeTS.Key()).Balance
		after := mustActor(f.ctx, t, f.client, recipient, claimTS.Key()).Balance
		deltas[i] = big.Sub(after, before)
		req.Equal(result.Amounts[i], deltas[i])
	}
	actor, state, streams := loadReward19(f.ctx, t, f.client, f.store, lookup.TipSet)
	f.requireAllocation(t, actor, state, streams)
	return solsticeClaimResult{tipset: claimTS, amounts: result.Amounts, deltas: deltas, state: state, streams: streams}
}

func (f *solsticeRewardLifecycle) stateAtOrAfter(t *testing.T, epoch abi.ChainEpoch) (*types.Actor, *reward19.State, *reward19.StreamsState) {
	t.Helper()
	ts := tipsetAtOrAfter(f.ctx, t, f.client, epoch)
	return loadReward19(f.ctx, t, f.client, f.store, ts.Key())
}

func (f *solsticeRewardLifecycle) requireAllocation(t *testing.T, actor *types.Actor, state *reward19.State, streams *reward19.StreamsState) {
	t.Helper()
	require.Equal(t, f.initialAllocation, rewardAllocationAt(t, actor, state, streams))
}

func rewardDeltas(start, end *reward19.State) solsticeRewardDeltas {
	total := big.Sub(end.TotalMintedReward, start.TotalMintedReward)
	service := big.Sub(end.TotalExplicitMinted, start.TotalExplicitMinted)
	burn := big.Sub(end.TotalBurnMinted, start.TotalBurnMinted)
	return solsticeRewardDeltas{total: total, service: service, burn: burn, miner: big.Sub(big.Sub(total, service), burn)}
}

func flatWeight(weight uint64) reward19.WeightRecord {
	return reward19.WeightRecord{VStart: weight, Floor: weight, Cap: weight}
}

func accruedShare(amount abi.TokenAmount, share uint64) abi.TokenAmount {
	return big.Div(big.Mul(amount, big.NewInt(int64(share))), big.NewInt(int64(reward19.Denom)))
}

func requireMessageSuccess(t *testing.T, lookup *api.MsgLookup) {
	t.Helper()
	require.True(t, lookup.Receipt.ExitCode.IsSuccess(), lookup.Receipt.ExitCode.String())
}

func requireWriteQueuedEvent(ctx context.Context, t *testing.T, node api.FullNode, pending reward19.PendingWrite) {
	t.Helper()
	req := require.New(t)
	var epochZero abi.ChainEpoch
	events, err := node.GetActorEventsRaw(ctx, &types.ActorEventFilter{
		Addresses:  []address.Address{builtin.RewardActorAddr},
		FromHeight: &epochZero,
	})
	req.NoError(err)
	expected := []types.EventEntry{
		eventEntry(t, 0x03, "$type", basicnode.NewString("write-queued")),
		eventEntry(t, 0x03, "op", basicnode.NewInt(int64(pending.Op))),
		eventEntry(t, 0x01, "effective-epoch", basicnode.NewInt(int64(pending.EffectiveEpoch))),
		eventEntry(t, 0x01, "payload", basicnode.NewBytes(pending.Payload)),
	}
	for _, event := range events {
		if len(event.Entries) > 2 &&
			event.Entries[0].Key == "$type" &&
			bytes.Equal(event.Entries[0].Value, expected[0].Value) &&
			event.Entries[2].Key == "effective-epoch" &&
			bytes.Equal(event.Entries[2].Value, expected[2].Value) {
			req.Equal(expected, event.Entries)
			return
		}
	}
	req.FailNow("write-queued event not found")
}

func eventEntry(t *testing.T, flags uint8, key string, value ipld.Node) types.EventEntry {
	t.Helper()
	encoded, err := ipld.Encode(value, dagcbor.Encode)
	require.NoError(t, err)
	return types.EventEntry{Flags: flags, Codec: uint64(multicodec.Cbor), Key: key, Value: encoded}
}
