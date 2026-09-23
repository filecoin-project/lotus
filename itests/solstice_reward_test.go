package itests

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/multiformats/go-multicodec"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-keccak"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	reward18 "github.com/filecoin-project/go-state-types/builtin/v18/reward"
	datacap19 "github.com/filecoin-project/go-state-types/builtin/v19/datacap"
	reward19 "github.com/filecoin-project/go-state-types/builtin/v19/reward"
	adt19 "github.com/filecoin-project/go-state-types/builtin/v19/util/adt"
	rewardMath "github.com/filecoin-project/go-state-types/builtin/v19/util/math"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/manifest"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v2api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/datacap"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	chainstate "github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestSolsticeRewardLifecycle drives f02's FIP-0118 reward streams through the governance
// contracts which run as UUPS proxies.
// Every SWA-gated f02 write arrives through the SWA proxy and stream 2's share map
// through the SRA proxy, covering:
//
//   - the v18 to v19 migration, award continuity, and circulating supply;
//   - the bootstrap weight ramp and the reward split it settles on;
//   - the v2 reward distribution API at head and after an in-block share update;
//   - the SWA's writes: the deferred-write queue, cancellation, registration, removal;
//   - the SRA's quarterly gate stepping stream 2's weight and submitting its share map;
//   - share settlement, wallet payouts, and tombstone claims on a stream the SWA registers;
//   - the SRA's registry reaching f02: a second orchestrator's share, its removal, a wallet swap;
//   - a datacap write that crosses the fork in the message pool.
func TestSolsticeRewardLifecycle(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	const (
		blockTime    = 100 * time.Millisecond
		upgradeEpoch = abi.ChainEpoch(180)
		activation   = upgradeEpoch + 1
		// The pre-migration starts this many epochs before the upgrade; setup must finish before it.
		preMigrationStart           = abi.ChainEpoch(15)
		timelock                    = abi.ChainEpoch(20)
		consensusWeightRampDuration = abi.ChainEpoch(81)
		splitSampleEpochs           = abi.ChainEpoch(10)

		sraActivationEpoch = uint64(activation)
		epochsPerQuarter   = uint64(90)
		// Volumes bind 60 epochs into the 90-epoch quarter, leaving 30 for RemoveOrchestrator.
		postPeriod         = uint64(50)
		verificationWindow = uint64(10)
		hold               = uint64(0) // no upgrade delay, fine for test deployment

		// The SWA initializes at quarter 1, so its first quarterlyGateCheck reads quarter 2.
		gateQuarter = uint64(2)

		// w3 has its own writer, for share and claim tests outside the SRA's rules.
		w3 = reward19.StreamID(3)
	)
	pct := reward19.Denom / 100

	quarterStart := abi.ChainEpoch(sraActivationEpoch + gateQuarter*epochsPerQuarter)
	postingEnd := quarterStart + abi.ChainEpoch(postPeriod)
	bindingEpoch := postingEnd + abi.ChainEpoch(verificationWindow)
	// The gate's first volume threshold is VOL_TARGET_ENTRY, 3500 USD in 1e18 fixed point.
	gateVolumeUSD := big.Mul(big.NewInt(4000), big.NewInt(1_000_000_000_000_000_000))
	// A passing step multiplies the threshold by VOL_TARGET_RATIO, putting it at 9450 USD.
	belowThresholdUSD := big.Mul(big.NewInt(5000), big.NewInt(1_000_000_000_000_000_000))
	// quarterlyGateCheck's first passing step sets stream 2 flat at (steps + 3) * 5%.
	steppedWeight := reward19.WeightRecord{
		VStart: 15 * pct, TStart: quarterStart, Floor: 15 * pct, Cap: 15 * pct,
	}

	owner1Key, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)
	owner2Key, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)
	orchestratorKey, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)
	w3WriterKey, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)
	recipient1Key, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)
	recipient2Key, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)
	deployerKey, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)
	orchestrator2Key, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)
	wallet1BKey, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)
	datacapSenderKey, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)

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
	}
	rampTotal := bootstrapParams.ConsensusWeight.VStart - bootstrapParams.ConsensusWeight.Floor
	// The service stream's bootstrap rise is one ninth of the consensus ramp (FIP-0118).
	req.Equal(rampTotal/9, bootstrapParams.ServiceWeight.Cap-bootstrapParams.ServiceWeight.VStart)
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

	// deployerKey's first four messages are the deploys below, at nonces 0 to 3, which fixes their
	// CREATE addresses; the migration resolves the f410 forms from its input state tree.
	deployerEth := kit.EthAddressForCreate(t, deployerKey.Address)
	createAddress := func(nonce uint64) address.Address {
		addr, err := kit.ComputeContractAddress(t, deployerEth, nonce).ToFilecoinAddress()
		req.NoError(err)
		return addr
	}
	sraImplF4, sraF4, swaImplF4, swaF4 := createAddress(0), createAddress(1), createAddress(2), createAddress(3)

	bootstrapParams.SWAActor = swaF4
	bootstrapParams.SRAActor = sraF4
	// The migration resolves the orchestrator's f1 address the same way.
	bootstrapParams.InitialOrchestrator = orchestratorKey.Address

	// The pre-migration runs on its own goroutine and only logs its failures, so record its result.
	var preMigrationRuns atomic.Int64
	var preMigrationErr atomic.Pointer[error]
	preMigration := filcns.PreUpgradeActorsV19With(bootstrapParams)

	client, miner, ens := kit.EnsembleMinimal(t,
		kit.MockProofs(),
		kit.ThroughRPC(),
		kit.Account(owner1Key, types.FromFil(100)),
		kit.Account(owner2Key, types.FromFil(100)),
		kit.Account(orchestratorKey, types.FromFil(100)),
		kit.Account(w3WriterKey, types.FromFil(100)),
		kit.Account(recipient1Key, types.FromFil(100)),
		kit.Account(recipient2Key, types.FromFil(100)),
		kit.Account(deployerKey, types.FromFil(100)),
		kit.Account(orchestrator2Key, types.FromFil(100)),
		kit.Account(wallet1BKey, types.FromFil(100)),
		kit.Account(datacapSenderKey, types.FromFil(100)),
		kit.UpgradeSchedule(
			stmgr.Upgrade{Network: network.Version28, Height: -1},
			stmgr.Upgrade{
				Network:   network.Version29,
				Height:    upgradeEpoch,
				Migration: filcns.UpgradeActorsV19With(bootstrapParams),
				PreMigrations: []stmgr.PreMigration{{
					PreMigration: func(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache,
						root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) error {
						err := preMigration(ctx, sm, cache, root, epoch, ts)
						preMigrationErr.Store(&err)
						preMigrationRuns.Add(1)
						return err
					},
					StartWithin:     preMigrationStart,
					DontStartWithin: 2,
					StopWithin:      2,
				}},
			},
		),
	)
	blockMiners := ens.InterconnectAll().BeginMining(blockTime)

	for _, account := range []*key.Key{
		owner1Key, owner2Key, orchestratorKey, w3WriterKey, recipient1Key, recipient2Key, deployerKey,
		orchestrator2Key, wallet1BKey, datacapSenderKey,
	} {
		_, err := client.WalletImport(ctx, &account.KeyInfo)
		req.NoError(err)
	}

	// deployer owns neither contract, so it doubles as the non-owner in the governance checks.
	deployer := deployerKey.Address
	owner1 := kit.EvmWordFromAddr(ctx, t, client, owner1Key.Address)
	owner2 := kit.EvmWordFromAddr(ctx, t, client, owner2Key.Address)
	orchestrator := kit.EvmWordFromAddr(ctx, t, client, orchestratorKey.Address)
	orchestratorID, err := client.StateLookupID(ctx, orchestratorKey.Address, types.EmptyTSK)
	req.NoError(err)
	w3WriterID, err := client.StateLookupID(ctx, w3WriterKey.Address, types.EmptyTSK)
	req.NoError(err)
	recipient1ID, err := client.StateLookupID(ctx, recipient1Key.Address, types.EmptyTSK)
	req.NoError(err)
	recipient2ID, err := client.StateLookupID(ctx, recipient2Key.Address, types.EmptyTSK)
	req.NoError(err)
	orchestrator2ID, err := client.StateLookupID(ctx, orchestrator2Key.Address, types.EmptyTSK)
	req.NoError(err)
	wallet1BID, err := client.StateLookupID(ctx, wallet1BKey.Address, types.EmptyTSK)
	req.NoError(err)

	// Each deploy must land on the address the bootstrap identifies.
	requireDeployedAt := func(what string, predicted address.Address, actorID uint64) address.Address {
		actual := solsticeIDAddress(t, actorID)
		resolved, err := client.StateLookupID(ctx, predicted, types.EmptyTSK)
		req.NoErrorf(err, "%s must deploy at its predicted address %s", what, predicted)
		req.Equalf(actual, resolved, "%s deployed at %s, not its predicted address %s", what, actual, predicted)
		return actual
	}

	// The seeded orchestrator wallet is where f02 pays w2, so it matches the bootstrap's InitialOrchestrator.
	sraImpl := client.EVM().DeployContract(ctx, deployer, solsticeCreationCode(t, "ServiceRewardsActor",
		solsticeAddress(owner1),
		solsticeAddress(owner2),
		solsticeAddress(orchestrator),
		solsticeAddress(orchestrator),
		solsticeUint64(epochsPerQuarter),
		solsticeUint64(postPeriod),
		solsticeUint64(verificationWindow),
		solsticeUint64(sraActivationEpoch),
		solsticeUint64(hold),
	))
	sraImplAddr := requireDeployedAt("SRA implementation", sraImplF4, sraImpl.ActorID)

	sraProxy := client.EVM().DeployContract(ctx, deployer, solsticeCreationCode(t, "ERC1967Proxy",
		solsticeAddress(kit.EvmWordBytes(sraImpl.EthAddress[:])),
		solsticeBytes(solsticeSelector(t, "ServiceRewardsActor", "initialize()")),
	))
	sraAddr := requireDeployedAt("SRA proxy", sraF4, sraProxy.ActorID)

	// The SWA reads volumes and quarter anchors from the SRA proxy.
	swaImpl := client.EVM().DeployContract(ctx, deployer, solsticeCreationCode(t, "StreamWeightActor",
		solsticeAddress(owner1),
		solsticeAddress(owner2),
		solsticeUint64(hold),
		solsticeAddress(kit.EvmWordBytes(sraProxy.EthAddress[:])),
	))
	swaImplAddr := requireDeployedAt("SWA implementation", swaImplF4, swaImpl.ActorID)

	swaProxy := client.EVM().DeployContract(ctx, deployer, solsticeCreationCode(t, "ERC1967Proxy",
		solsticeAddress(kit.EvmWordBytes(swaImpl.EthAddress[:])),
		solsticeBytes(solsticeSelector(t, "StreamWeightActor", "initialize()")),
	))
	swaAddr := requireDeployedAt("SWA proxy", swaF4, swaProxy.ActorID)

	t.Logf("SRA implementation %s proxy %s; SWA implementation %s proxy %s",
		sraImplAddr, sraAddr, swaImplAddr, swaAddr)

	isAdmitted := solsticeSelector(t, "ServiceRewardsActor", "isAdmitted(address)")
	orchestratorCount := solsticeSelector(t, "ServiceRewardsActor", "orchestratorCount()")
	removeOrchestrator := solsticeSelector(t, "ServiceRewardsActor", "removeOrchestrator(address)")

	req.True(solsticeBool(t, solsticeInvoke(ctx, t, client, deployer, sraAddr, isAdmitted, orchestrator)),
		"initializing the SRA proxy must seat the orchestrator the implementation carries")
	req.Equal(kit.EvmWordUint64(1),
		solsticeWord(t, solsticeInvoke(ctx, t, client, deployer, sraAddr, orchestratorCount)),
		"the seeded orchestrator is the registry's only row")

	implCall := solsticeInvoke(ctx, t, client, owner1Key.Address, sraImplAddr, removeOrchestrator, orchestrator)
	req.False(implCall.Receipt.ExitCode.IsSuccess(),
		"the SRA implementation disabled its initializers, so it has no owners and an owner-gated call must revert")

	nonGovCall := solsticeInvoke(ctx, t, client, deployer, sraAddr, removeOrchestrator, orchestrator)
	req.False(nonGovCall.Receipt.ExitCode.IsSuccess(), "a non-owner must not reach SRA governance")

	// Nonce 0 runs pre-fork; nonce 2 waits in the pool behind the gap at nonce 1, filled after it.
	datacapTransfer := &datacap19.TransferParams{
		To:     builtin.VerifiedRegistryActorAddr,
		Amount: big.Mul(big.NewInt(1<<30), builtin.TokenPrecision),
	}
	datacapControl := solsticeWait(ctx, t, client, solsticePushMessage(ctx, t, client,
		datacapSenderKey.Address, 0, builtin.DatacapActorAddr, datacap.Methods.TransferExported, datacapTransfer))

	setupHead, err := client.ChainHead(ctx)
	req.NoError(err)
	t.Logf("deployment and governance complete at epoch %d", setupHead.Height())
	req.Lessf(setupHead.Height(), upgradeEpoch-preMigrationStart,
		"setup must finish before epoch %d, when the pre-migration may start",
		upgradeEpoch-preMigrationStart)

	store := cbor.NewCborStore(blockstore.NewAPIBlockstore(client))
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch-2))
	datacapInFlight := solsticePushMessage(ctx, t, client,
		datacapSenderKey.Address, 2, builtin.DatacapActorAddr, datacap.Methods.TransferExported, datacapTransfer)

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+8))
	// Filling the gap releases the pre-fork message.
	solsticePushMessage(ctx, t, client, datacapSenderKey.Address, 1, datacapSenderKey.Address, builtin.MethodSend, nil)

	preTS := tipsetAtOrBefore(ctx, t, client, upgradeEpoch)
	req.Equal(upgradeEpoch, preTS.Height(), "upgrade epoch must contain a tipset")
	preActor, err := client.StateGetActor(ctx, builtin.RewardActorAddr, preTS.Key())
	req.NoError(err)
	var pre reward18.State
	req.NoError(store.Get(ctx, preActor.Head, &pre))
	// v18 issuance totals, compared with the v19 constants later.
	preSimpleTotal := pre.SimpleTotal
	preBaselineTotal := pre.BaselineTotal
	initialAllocation := big.Add(preActor.Balance, pre.TotalStoragePowerReward)

	// The fork runs while computing the activation tipset, so its parent state is the migration input.
	activationTS := kit.TipsetAtOrAfter(ctx, t, client, activation)
	migrationInputActor, err := client.StateGetActor(ctx, builtin.RewardActorAddr, activationTS.Key())
	req.NoError(err)
	var migrationInput reward18.State
	req.NoError(store.Get(ctx, migrationInputActor.Head, &migrationInput))

	req.Positive(preMigrationRuns.Load(), "the nv29 pre-migration must have run")
	req.NoError(*preMigrationErr.Load(), "the nv29 pre-migration must warm the cache, not fail")

	// Replays the fork on the same root with nothing executed after it: the exact migration output.
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
		blockMiners:         blockMiners,
		store:               store,
		pct:                 pct,
		upgradeEpoch:        upgradeEpoch,
		activation:          activation,
		timelock:            timelock,
		bootstrapSlope:      bootstrapSlope,
		splitStart:          splitStartEpoch,
		splitEnd:            splitEndEpoch,
		bootstrap:           bootstrapParams,
		deployer:            deployer,
		owner1:              owner1Key.Address,
		owner2:              owner2Key.Address,
		sraAddr:             sraAddr,
		swaAddr:             swaAddr,
		orchestrator:        orchestratorKey.Address,
		orchestratorID:      orchestratorID,
		orchestrator2:       orchestrator2Key.Address,
		orchestrator2ID:     orchestrator2ID,
		wallet1B:            wallet1BKey.Address,
		wallet1BID:          wallet1BID,
		datacapControl:      datacapControl,
		datacapInFlight:     datacapInFlight,
		w3WriterKey:         w3WriterKey,
		w3WriterID:          w3WriterID,
		recipients:          []address.Address{recipient1ID, recipient2ID},
		newStream:           w3,
		gateQuarter:         gateQuarter,
		gateVolumeUSD:       gateVolumeUSD,
		belowThresholdUSD:   belowThresholdUSD,
		epochsPerQuarter:    abi.ChainEpoch(epochsPerQuarter),
		quarterStart:        quarterStart,
		postingEnd:          postingEnd,
		bindingEpoch:        bindingEpoch,
		steppedWeight:       steppedWeight,
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
	t.Run("a datacap transfer crossing the fork takes the new rules", lifecycle.testInFlightDatacapTransfer)
	t.Run("circulating supply continuity", lifecycle.testCirculatingSupplyContinuity)
	t.Run("reward total constants replace stored state", lifecycle.testRewardTotalConstants)
	t.Run("sloped bootstrap weight", lifecycle.testSlopedBootstrapWeight)
	t.Run("reward split economics", lifecycle.testRewardSplitEconomics)
	t.Run("reward distribution API at head without a child", lifecycle.testRewardDistributionAtHead)
	t.Run("queue controls and write event", lifecycle.testQueueControlsAndEvent)
	t.Run("deferred weight schedule applies across null rounds", lifecycle.testDeferredWeightSchedule)
	t.Run("quarterly gate step and share submission", lifecycle.testQuarterlyGateAndShares)
	t.Run("failed gate holds the stepped weight", lifecycle.testFailedGate)
	// The w3 phases fill the wait for the missed-post quarter to bind and leave stream 2 as the only
	// explicit stream by the time it runs.
	t.Run("stream registration", lifecycle.testStreamRegistration)
	t.Run("share settlement and wallet payouts", lifecycle.testShareSettlementAndWalletPayouts)
	t.Run("remove stream tombstone claim", lifecycle.testRemoveStreamTombstoneClaim)
	t.Run("missed volume post submits no shares", lifecycle.testMissedVolumePost)
	t.Run("second orchestrator shares, removal and wallet replacement", lifecycle.testSecondOrchestrator)
}

// TestSolsticeUpgradeAcrossNullRound runs the NV29 migration when neither the upgrade epoch nor the
// activation epoch has a tipset. The fork still applies at its scheduled epoch, the bootstrap
// record anchors one epoch after it, and awards resume on the other side of the gap.
func TestSolsticeUpgradeAcrossNullRound(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	const (
		blockTime    = 100 * time.Millisecond
		upgradeEpoch = abi.ChainEpoch(50)
		activation   = upgradeEpoch + 1
	)

	// The consensus-only bootstrap: one implicit stream at the whole denominator.
	migration := filcns.UpgradeActorsV19With(buildconstants.NeutralSolsticeRewardBootstrapParams)
	var migrationRuns, migrationEpoch atomic.Int64

	client, _, ens := kit.EnsembleMinimal(t,
		kit.MockProofs(),
		kit.UpgradeSchedule(
			stmgr.Upgrade{Network: network.Version28, Height: -1},
			stmgr.Upgrade{
				Network: network.Version29,
				Height:  upgradeEpoch,
				Migration: func(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache,
					cb stmgr.ExecMonitor, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
					migrationRuns.Add(1)
					migrationEpoch.Store(int64(epoch))
					return migration(ctx, sm, cache, cb, root, epoch, ts)
				},
			},
		),
	)
	blockMiners := ens.InterconnectAll().BeginMining(blockTime)

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch-10))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	req.Lessf(head.Height(), upgradeEpoch,
		"chain head %d has reached the upgrade epoch %d, too late to leave it null", head.Height(), upgradeEpoch)
	for _, blockMiner := range blockMiners {
		blockMiner.InjectNulls(activation + 1 - head.Height())
	}
	client.WaitTillChain(ctx, kit.HeightAtLeast(activation+10))

	lastPre := tipsetAtOrBefore(ctx, t, client, upgradeEpoch)
	req.Lessf(lastPre.Height(), upgradeEpoch, "upgrade epoch %d must be a null round", upgradeEpoch)
	firstAfter := kit.TipsetAtOrAfter(ctx, t, client, activation)
	req.Greaterf(firstAfter.Height(), activation, "activation epoch %d must be a null round", activation)
	t.Logf("null rounds span epochs %d to %d", lastPre.Height()+1, firstAfter.Height()-1)

	req.Positive(migrationRuns.Load(), "the nv29 migration must run")
	req.Equal(int64(upgradeEpoch), migrationEpoch.Load(),
		"the migration must run at its scheduled epoch, not the first epoch with a tipset")

	store := cbor.NewCborStore(blockstore.NewAPIBlockstore(client))
	// The null rounds and the fork apply while computing firstAfter, so its parent state is pre-fork.
	preActor, err := client.StateGetActor(ctx, builtin.RewardActorAddr, firstAfter.Key())
	req.NoError(err)
	var pre reward18.State
	req.NoError(store.Get(ctx, preActor.Head, &pre))

	// Replays the fork on that state with nothing executed after it (i.e. the exact migration output).
	computed, err := client.StateCompute(ctx, activation, nil, lastPre.Key())
	req.NoError(err)
	migrationTree, err := chainstate.LoadStateTree(store, computed.Root)
	req.NoError(err)
	migrationActor, err := migrationTree.GetActor(builtin.RewardActorAddr)
	req.NoError(err)
	expectedCode, ok := actors.GetActorCodeID(actorstypes.Version19, manifest.RewardKey)
	req.True(ok)
	req.Equal(expectedCode, migrationActor.Code)
	var migrated reward19.State
	req.NoError(store.Get(ctx, migrationActor.Head, &migrated))
	adtStore := adt19.WrapStore(ctx, store)
	migratedStreams, err := migrated.LoadStreams(adtStore)
	req.NoError(err)
	_, invariantMessages := reward19.CheckStateInvariants(&migrated, adtStore, lastPre.Height(), migrationActor.Balance)
	req.Empty(invariantMessages.Messages())

	// The bootstrap anchors on the scheduled epoch, not on the first epoch that has a tipset.
	req.Len(migratedStreams.Streams, 1)
	req.Equal(reward19.StreamID(1), migratedStreams.Streams[0].ID)
	req.Nil(migratedStreams.Streams[0].Distribution, "the consensus stream is implicit")
	req.Equal(reward19.WeightRecord{
		VStart: reward19.Denom, TStart: activation, Floor: reward19.Denom, Cap: reward19.Denom,
	}, migratedStreams.Streams[0].Weight)
	req.Equal(pre.TotalStoragePowerReward, migrated.TotalMintedReward)
	req.Equal(big.Zero(), migrated.TotalBurnMinted)
	req.Equal(big.Zero(), migrated.TotalExplicitMinted)

	// Awards resume on the first tipset after the gap, and FilMined keeps tracking them.
	resumedTS := kit.TipsetAtOrAfter(ctx, t, client, firstAfter.Height()+1)
	version, err := client.StateNetworkVersion(ctx, resumedTS.Key())
	req.NoError(err)
	req.Equal(network.Version29, version)
	client.WaitTillChain(ctx, kit.HeightAtLeast(resumedTS.Height()+9))
	laterTS := kit.TipsetAtOrAfter(ctx, t, client, resumedTS.Height()+8)
	_, resumed, _ := kit.LoadReward19(ctx, t, client, store, resumedTS.Key())
	_, later, _ := kit.LoadReward19(ctx, t, client, store, laterTS.Key())
	req.Positive(big.Cmp(resumed.TotalMintedReward, migrated.TotalMintedReward),
		"the first award after the null rounds must mint")
	req.Positive(big.Cmp(later.TotalMintedReward, resumed.TotalMintedReward), "awards must continue")
	req.Equal(big.Zero(), later.TotalBurnMinted, "a consensus-only bootstrap pays the miner everything")
	req.Equal(big.Zero(), later.TotalExplicitMinted)

	resumedSupply, err := client.StateVMCirculatingSupplyInternal(ctx, resumedTS.Key())
	req.NoError(err)
	laterSupply, err := client.StateVMCirculatingSupplyInternal(ctx, laterTS.Key())
	req.NoError(err)
	req.Equal(big.Sub(later.TotalMintedReward, resumed.TotalMintedReward),
		big.Sub(laterSupply.FilMined, resumedSupply.FilMined))
}

func tipsetAtOrBefore(ctx context.Context, t *testing.T, node api.FullNode, target abi.ChainEpoch) *types.TipSet {
	t.Helper()
	req := require.New(t)
	ts, err := node.ChainGetTipSetByHeight(ctx, target, types.EmptyTSK)
	req.NoError(err)
	req.LessOrEqual(ts.Height(), target)
	return ts
}

// adjacentReward19States finds a single-epoch transition so reward recomputation doesn't straddle nulls.
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
		_, previous, _ := kit.LoadReward19(ctx, t, node, store, parent.Key())
		_, current, _ := kit.LoadReward19(ctx, t, node, store, ts.Key())
		if current.Epoch == previous.Epoch+1 {
			return previous, current
		}
	}
	req.FailNow("no adjacent v19 reward states", "epoch %d through head %d", start, head.Height())
	return nil, nil
}

// computeRewardWithTotals evaluates the v19 formula with caller-supplied issuance totals.
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

// explicitServiceLiabilities sums stream accruals (net of claimed) plus tombstone payables held by f02.
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

func rewardAllocationAt(t *testing.T, actor *types.Actor, state *reward19.State, streams *reward19.StreamsState) abi.TokenAmount {
	t.Helper()
	explicitLiabilities := explicitServiceLiabilities(t, state, streams)
	remainingReserve := big.Sub(actor.Balance, explicitLiabilities)
	return big.Add(state.TotalMintedReward, remainingReserve)
}

// requireShareWithinAtto checks part/total ≈ share/Denom with an attoFIL rounding allowance.
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

func streamByID(t *testing.T, streams *reward19.StreamsState, id reward19.StreamID) reward19.Stream {
	t.Helper()
	for _, stream := range streams.Streams {
		if stream.ID == id {
			return stream
		}
	}
	require.FailNowf(t, "stream is not live", "stream %d", id)
	return reward19.Stream{}
}

// accrualOf returns the current period's gross accrual for one explicit stream.
func accrualOf(t *testing.T, state *reward19.State, id reward19.StreamID) abi.TokenAmount {
	t.Helper()
	for _, accrual := range state.Accrued {
		if accrual.ID == id {
			return accrual.Amount
		}
	}
	require.FailNowf(t, "stream has no accrual", "stream %d", id)
	return big.Zero()
}

type solsticeRewardLifecycle struct {
	ctx         context.Context
	client      *kit.TestFullNode
	miner       *kit.TestMiner
	blockMiners []*kit.BlockMiner
	store       cbor.IpldStore

	pct            uint64
	upgradeEpoch   abi.ChainEpoch
	activation     abi.ChainEpoch
	timelock       abi.ChainEpoch
	bootstrapSlope uint64
	splitStart     abi.ChainEpoch
	splitEnd       abi.ChainEpoch
	bootstrap      buildconstants.SolsticeRewardBootstrapParams

	deployer        address.Address
	owner1          address.Address
	owner2          address.Address
	sraAddr         address.Address
	swaAddr         address.Address
	orchestrator    address.Address
	orchestratorID  address.Address
	orchestrator2   address.Address
	orchestrator2ID address.Address
	wallet1B        address.Address
	wallet1BID      address.Address
	w3WriterKey     *key.Key
	w3WriterID      address.Address
	recipients      []address.Address
	newStream       reward19.StreamID

	datacapControl  *api.MsgLookup
	datacapInFlight cid.Cid

	gateQuarter       uint64
	gateVolumeUSD     big.Int
	belowThresholdUSD big.Int
	epochsPerQuarter  abi.ChainEpoch
	quarterStart      abi.ChainEpoch
	postingEnd        abi.ChainEpoch
	bindingEpoch      abi.ChainEpoch
	steppedWeight     reward19.WeightRecord

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

// testMigrationAndAwardContinuity checks v18 to v19 field mapping and that awards continue post-migration.
func (f *solsticeRewardLifecycle) testMigrationAndAwardContinuity(t *testing.T) {
	req := require.New(t)
	expectedCode, ok := actors.GetActorCodeID(actorstypes.Version19, manifest.RewardKey)
	req.True(ok)
	req.Equal(expectedCode, f.migrationActor.Code)

	// Input and output are the same epoch's state: the v18 parent state and its migrated form.
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
	req.Equal(f.swaAddr, f.migrated.SWAActor)
	req.Len(f.migratedStreams.Streams, 2)
	req.Empty(f.migratedStreams.PendingWritesQueue)
	req.Empty(f.migratedStreams.Tombstones)
	req.Equal(f.activation, f.migratedStreams.Streams[0].Weight.TStart)
	req.Equal(f.activation, f.migratedStreams.Streams[1].Weight.TStart)
	req.Equal(f.bootstrap.ConsensusWeight.VStart, f.migratedStreams.Streams[0].Weight.VStart)
	req.Equal(f.bootstrap.ServiceWeight.VStart, f.migratedStreams.Streams[1].Weight.VStart)
	req.Equal(-int64(f.bootstrapSlope), f.migratedStreams.Streams[0].Weight.Slope)
	req.Equal(int64(f.bootstrapSlope), f.migratedStreams.Streams[1].Weight.Slope)
	req.Equal(reward19.StreamID(1), f.migratedStreams.Streams[0].ID)
	req.Equal(reward19.StreamID(2), f.migratedStreams.Streams[1].ID)
	req.Nil(f.migratedStreams.Streams[0].Distribution, "the consensus stream is implicit")
	req.NotNil(f.migratedStreams.Streams[1].Distribution)
	req.Equal(f.sraAddr, f.migratedStreams.Streams[1].Distribution.Writer)
	req.Equal([]reward19.RecipientShare{{Recipient: f.orchestratorID, Share: reward19.Denom}}, f.migratedStreams.Streams[1].Distribution.Shares)

	laterTS := kit.TipsetAtOrAfter(f.ctx, t, f.client, f.upgradeEpoch+8)
	laterActor, later, laterStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, laterTS.Key())
	req.Positive(big.Cmp(later.TotalMintedReward, f.migrated.TotalMintedReward))
	req.Positive(big.Cmp(later.TotalExplicitMinted, f.migrated.TotalExplicitMinted))
	f.requireAllocation(t, laterActor, later, laterStreams)

	// The sole miner receives all issuance minus burn and explicit accrual, plus gas rewards.
	consensusMinted := big.Sub(
		big.Sub(later.TotalMintedReward, f.migrated.TotalMintedReward),
		big.Add(
			big.Sub(later.TotalExplicitMinted, f.migrated.TotalExplicitMinted),
			big.Sub(later.TotalBurnMinted, f.migrated.TotalBurnMinted)))
	req.Positive(consensusMinted.Sign())
	preMiner := kit.MustActor(f.ctx, t, f.client, f.miner.ActorAddr, f.activationTS.Key())
	postMiner := kit.MustActor(f.ctx, t, f.client, f.miner.ActorAddr, laterTS.Key())
	req.GreaterOrEqual(big.Cmp(big.Sub(postMiner.Balance, preMiner.Balance), consensusMinted), 0,
		"the miner must receive at least the consensus share minted since the migration")
}

// testInFlightDatacapTransfer checks a datacap write that crosses the boundary in the message pool
// executes under NV29's refusal, with the pre-fork failure for comparison.
func (f *solsticeRewardLifecycle) testInFlightDatacapTransfer(t *testing.T) {
	req := require.New(t)
	controlTS, err := f.client.ChainGetTipSet(f.ctx, f.datacapControl.TipSet)
	req.NoError(err)
	req.Lessf(controlTS.Height(), f.activation, "the control must execute before the fork at epoch %d", f.activation)
	req.False(f.datacapControl.Receipt.ExitCode.IsSuccess(), "an account holding no datacap cannot transfer any")
	controlReplay, err := f.client.StateReplay(f.ctx, types.EmptyTSK, f.datacapControl.Message)
	req.NoError(err)
	req.NotContains(controlReplay.Error, "FIP-0118", "NV28's datacap actor refuses over the balance")

	inFlight := solsticeWait(f.ctx, t, f.client, f.datacapInFlight)
	inFlightTS, err := f.client.ChainGetTipSet(f.ctx, inFlight.TipSet)
	req.NoError(err)
	executionTS, err := f.client.ChainGetTipSet(f.ctx, inFlightTS.Parents())
	req.NoError(err)
	req.GreaterOrEqualf(executionTS.Height(), f.activation,
		"the in-flight message executed at epoch %d, before the fork", executionTS.Height())
	req.Equal(exitcode.ErrForbidden, inFlight.Receipt.ExitCode)
	req.NotEqual(f.datacapControl.Receipt.ExitCode, inFlight.Receipt.ExitCode,
		"the pre-fork refusal must differ from FIP-0118's")
	inFlightReplay, err := f.client.StateReplay(f.ctx, types.EmptyTSK, inFlight.Message)
	req.NoError(err)
	req.Contains(inFlightReplay.Error, "FIP-0118")
}

// testCirculatingSupplyContinuity proves FilMined tracks TotalMintedReward across and after migration.
func (f *solsticeRewardLifecycle) testCirculatingSupplyContinuity(t *testing.T) {
	req := require.New(t)
	postMigrationTS := kit.TipsetAtOrAfter(f.ctx, t, f.client, f.activationTS.Height()+1)
	postActor, postMigration, _ := kit.LoadReward19(f.ctx, t, f.client, f.store, postMigrationTS.Key())
	preSupply, err := f.client.StateVMCirculatingSupplyInternal(f.ctx, f.activationTS.Key())
	req.NoError(err)
	postSupply, err := f.client.StateVMCirculatingSupplyInternal(f.ctx, postMigrationTS.Key())
	req.NoError(err)
	mined := big.Sub(postSupply.FilMined, preSupply.FilMined)
	req.Equal(big.Sub(postMigration.TotalMintedReward, f.migrationInput.TotalStoragePowerReward), mined)
	// Explicit accrual stays in f02 until claimed; the rest of what is mined leaves at once.
	req.Equal(
		big.Sub(mined, big.Sub(postMigration.TotalExplicitMinted, f.migrated.TotalExplicitMinted)),
		big.Sub(f.migrationInputActor.Balance, postActor.Balance),
	)

	laterEpoch := postMigrationTS.Height() + 7
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(laterEpoch+1))
	laterTS := kit.TipsetAtOrAfter(f.ctx, t, f.client, laterEpoch)
	laterActor, later, _ := kit.LoadReward19(f.ctx, t, f.client, f.store, laterTS.Key())
	laterSupply, err := f.client.StateVMCirculatingSupplyInternal(f.ctx, laterTS.Key())
	req.NoError(err)
	mined = big.Sub(laterSupply.FilMined, postSupply.FilMined)
	req.Equal(big.Sub(later.TotalMintedReward, postMigration.TotalMintedReward), mined)
	req.Equal(
		big.Sub(mined, big.Sub(later.TotalExplicitMinted, postMigration.TotalExplicitMinted)),
		big.Sub(postActor.Balance, laterActor.Balance),
	)
}

// testRewardTotalConstants distinguishes the v19 issuance constants from the v18 state totals.
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

// testSlopedBootstrapWeight observes the consensus ramp reducing the miner share by at least slope*epochs.
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
	lowerBound := big.Sub(minimumDropValue, earlyRoundingSlack)
	req.GreaterOrEqual(big.Sub(actualDrop, lowerBound).Sign(), 0,
		"miner share drop %s over %d epochs is below the slope's floor %s", actualDrop, separation, lowerBound)
}

// testRewardSplitEconomics checks settled bootstrap weights split gross issuance among miner, service, and burn.
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

func (f *solsticeRewardLifecycle) testRewardDistributionAtHead(t *testing.T) {
	req := require.New(t)
	for _, blockMiner := range f.blockMiners {
		blockMiner.Pause()
	}
	defer func() {
		for _, blockMiner := range f.blockMiners {
			blockMiner.Restart()
		}
	}()
	// Pause stops scheduling, but the last asynchronous mining request may still
	// be publishing. Complete a synchronous request before selecting the head.
	f.blockMiners[0].MineUntilBlock(f.ctx, f.client, nil)

	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	_, before, _ := kit.LoadReward19(f.ctx, t, f.client, f.store, head.Key())
	result, err := f.client.V2.StateRewardDistribution(f.ctx, types.TipSetSelectors.Latest)
	req.NoError(err)
	req.Equal(head.Key(), result.TipSetKey)
	req.Equal(head.Height(), result.Height)
	req.Equal(reward19.Denom, result.Denom)
	req.Len(result.Blocks, len(head.Blocks()))
	requireRewardAPIConservation(t, result)

	// No child can commit this result while mining is paused. Independently
	// compute the selected head's output and compare its minted-counter changes.
	computed, err := f.client.StateCompute(f.ctx, head.Height(), nil, head.Key())
	req.NoError(err)
	tree, err := chainstate.LoadStateTree(f.store, computed.Root)
	req.NoError(err)
	actor, err := tree.GetActor(builtin.RewardActorAddr)
	req.NoError(err)
	var after reward19.State
	req.NoError(f.store.Get(f.ctx, actor.Head, &after))
	delta := rewardDeltas(before, &after)
	req.Equal(delta.total, result.Totals.MintedReward)
	req.Equal(delta.miner, result.Totals.MinerReward)
	req.Equal(delta.service, result.Totals.ExplicitReward)
	req.Equal(delta.burn, result.Totals.BurnAllocation)
	req.Positive(result.Totals.MintedReward.Sign())
	req.Equal(big.Add(result.Totals.MinerReward, result.Totals.MessageReward), result.Totals.MinerPaid)
	req.Equal(result.Totals.BurnAllocation, result.Totals.BurnPaid)

	for i, block := range result.Blocks {
		header := head.Blocks()[i]
		req.Equal(header.Cid(), block.Block)
		req.Equal(header.Miner, block.Miner)
		req.Equal(header.ElectionProof.WinCount, block.WinCount)
		req.Len(block.Streams, 2)
		req.Equal(uint64(1), block.Streams[0].ID)
		req.Equal(reward.ComputeWeight(f.migratedStreams.Streams[0].Weight, head.Height()), block.Streams[0].Weight)
		req.Nil(block.Streams[0].Distribution)
		req.Equal(block.Amounts.MinerReward, block.Streams[0].Amount)
		service := block.Streams[1]
		req.Equal(uint64(2), service.ID)
		req.Equal(reward.ComputeWeight(f.migratedStreams.Streams[1].Weight, head.Height()), service.Weight)
		req.NotNil(service.Distribution)
		req.Equal(f.sraAddr, service.Distribution.Writer)
		req.Len(service.Distribution.Recipients, 1)
		recipient := service.Distribution.Recipients[0]
		req.Equal(f.orchestratorID, recipient.Recipient)
		req.Equal(reward19.Denom, recipient.Share)
		req.Equal(service.Amount, recipient.EarnedAmount)
	}

	pinned, err := f.client.V2.StateRewardDistribution(f.ctx, types.TipSetSelectors.Key(head.Key()))
	req.NoError(err)
	req.Equal(result, pinned)

	stillHead, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	req.Equal(head.Key(), stillHead.Key(), "the query must work before a child tipset exists")

	legacy, err := f.client.V2.StateRewardDistribution(f.ctx, types.TipSetSelectors.Key(f.preTS.Key()))
	req.Error(err, "v18 has no reward stream distribution")
	req.Nil(legacy)
}

func requireRewardAPIConservation(t *testing.T, result *v2api.RewardDistribution) {
	t.Helper()
	req := require.New(t)
	for _, block := range result.Blocks {
		req.Equal(block.Amounts.MintedReward,
			big.Sum(block.Amounts.MinerReward, block.Amounts.ExplicitReward, block.Amounts.BurnAllocation))
		weights := block.BurnWeight
		gross, streamBurn := big.Zero(), big.Zero()
		for _, stream := range block.Streams {
			weights += stream.Weight
			gross = big.Add(gross, stream.Amount)
			if stream.Distribution == nil {
				continue
			}
			distribution := stream.Distribution
			shares, earned := distribution.BurnShare, big.Zero()
			for _, recipient := range distribution.Recipients {
				shares += recipient.Share
				earned = big.Add(earned, recipient.EarnedAmount)
			}
			req.Equal(result.Denom, shares)
			req.Equal(stream.Amount, big.Sum(earned, distribution.BurnAmount, distribution.RoundingAdjustment))
			streamBurn = big.Add(streamBurn, distribution.BurnAmount)
		}
		req.Equal(result.Denom, weights)
		req.Equal(block.Amounts.MintedReward, big.Sub(big.Add(gross, block.Amounts.BurnAllocation), streamBurn))
	}
	for _, field := range []func(v2api.RewardAmounts) abi.TokenAmount{
		func(a v2api.RewardAmounts) abi.TokenAmount { return a.MintedReward },
		func(a v2api.RewardAmounts) abi.TokenAmount { return a.MinerReward },
		func(a v2api.RewardAmounts) abi.TokenAmount { return a.MessageReward },
		func(a v2api.RewardAmounts) abi.TokenAmount { return a.ExplicitReward },
		func(a v2api.RewardAmounts) abi.TokenAmount { return a.BurnAllocation },
		func(a v2api.RewardAmounts) abi.TokenAmount { return a.MinerPaid },
		func(a v2api.RewardAmounts) abi.TokenAmount { return a.BurnPaid },
	} {
		sum := big.Zero()
		for _, block := range result.Blocks {
			sum = big.Add(sum, field(block.Amounts))
		}
		req.Equal(sum, field(result.Totals))
	}
}

// testQueueControlsAndEvent checks SWA auth, occupied-slot rejection, cancellation, and write-queued event.
func (f *solsticeRewardLifecycle) testQueueControlsAndEvent(t *testing.T) {
	req := require.New(t)
	updates := []reward19.WeightRecordUpdate{
		{ID: 1, Weight: flatWeight(65 * f.pct)},
		{ID: 2, Weight: flatWeight(10 * f.pct)},
	}
	beforeTS, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	_, _, beforeStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, beforeTS.Key())
	beforeWeights := []reward19.WeightRecord{beforeStreams.Streams[0].Weight, beforeStreams.Streams[1].Weight}

	// A single owner cancelling an empty slot is a no-op that proves proxy ownership and f02's
	// SWAActor.
	req.False(f.cancelPendingWeight(t, f.deployer, reward19.PendingWriteOpSetWeightRecords).Receipt.ExitCode.IsSuccess(),
		"a non-owner must not reach SWA governance")
	kit.RequireMessageSuccess(t, f.cancelPendingWeight(t, f.owner1, reward19.PendingWriteOpSetWeightRecords))

	unauthorized := f.sendRewardMessage(t, f.w3WriterKey.Address, builtin.MethodsReward.SetWeightRecordsExported,
		&reward19.SetWeightRecordsParams{Updates: updates})
	req.False(unauthorized.Receipt.ExitCode.IsSuccess(), "only the SWA may write weight records")

	queuedLookup := f.swaSetWeightRecords(t, updates)
	kit.RequireMessageSuccess(t, queuedLookup)
	queuedActor, queued, queuedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, queuedLookup.TipSet)
	f.requireAllocation(t, queuedActor, queued, queuedStreams)
	req.Len(queuedStreams.PendingWritesQueue, 1)
	pending := queuedStreams.PendingWritesQueue[0]
	req.Equal(reward19.PendingWriteOpSetWeightRecords, pending.Op)

	firstCollision, secondCollision := f.unanimousInvoke(t, f.swaAddr, solsticeSetWeightRecords(t), solsticeWeightUpdatesInput(updates))
	rejected := 0
	for _, lookup := range []*api.MsgLookup{firstCollision, secondCollision} {
		if !lookup.Receipt.ExitCode.IsSuccess() {
			rejected++
		}
	}
	req.Equal(1, rejected, "an occupied queue slot must reject the approval that reaches unanimity")
	_, _, collisionStreams := kit.LoadReward19(f.ctx, t, f.client, f.store,
		solsticeLaterLookup(firstCollision, secondCollision).TipSet)
	req.Len(collisionStreams.PendingWritesQueue, 1)
	req.Equal(pending, collisionStreams.PendingWritesQueue[0], "a rejected write leaves the queued one intact")

	cancel := f.cancelPendingWeight(t, f.owner1, reward19.PendingWriteOpSetWeightRecords)
	kit.RequireMessageSuccess(t, cancel)
	cancelActor, cancelled, cancelledStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, cancel.TipSet)
	f.requireAllocation(t, cancelActor, cancelled, cancelledStreams)
	req.Empty(cancelledStreams.PendingWritesQueue)
	req.Equal(beforeWeights[0], cancelledStreams.Streams[0].Weight)
	req.Equal(beforeWeights[1], cancelledStreams.Streams[1].Weight)
	requireWriteQueuedEvent(f.ctx, t, f.client, pending)

	// The revert keeps the other owner's approval on record; a successful veto proves it is still
	// there.
	kit.RequireMessageSuccess(t, solsticeInvoke(f.ctx, t, f.client, f.owner1, f.swaAddr,
		solsticeSelector(t, "StreamWeightActor", "veto(bytes32)"),
		solsticeTaskID(solsticeSetWeightRecords(t), solsticeWeightUpdatesInput(updates))))

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(pending.EffectiveEpoch+2))
	_, _, afterStreams := f.stateAtOrAfter(t, pending.EffectiveEpoch+1)
	req.Empty(afterStreams.PendingWritesQueue)
	req.Equal(beforeWeights[0], afterStreams.Streams[0].Weight)
	req.Equal(beforeWeights[1], afterStreams.Streams[1].Weight)
}

// testDeferredWeightSchedule proves a queued schedule stays inert until due, survives null rounds
// over its effective epoch, then controls reward splits.
func (f *solsticeRewardLifecycle) testDeferredWeightSchedule(t *testing.T) {
	req := require.New(t)
	consensusWeight := flatWeight(70 * f.pct)
	serviceWeight := flatWeight(10 * f.pct)
	lookup := f.swaSetWeightRecords(t, []reward19.WeightRecordUpdate{
		{ID: 1, Weight: consensusWeight}, {ID: 2, Weight: serviceWeight},
	})
	kit.RequireMessageSuccess(t, lookup)

	receiptTS, err := f.client.ChainGetTipSet(f.ctx, lookup.TipSet)
	req.NoError(err)
	parentTS, err := f.client.ChainGetTipSet(f.ctx, receiptTS.Parents())
	req.NoError(err)
	parentActor, parentState, parentStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, parentTS.Key())
	queuedActor, queuedState, queuedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, receiptTS.Key())
	f.requireAllocation(t, parentActor, parentState, parentStreams)
	f.requireAllocation(t, queuedActor, queuedState, queuedStreams)
	req.Equal(parentStreams.Streams[0].Weight, queuedStreams.Streams[0].Weight)
	req.Equal(parentStreams.Streams[1].Weight, queuedStreams.Streams[1].Weight)
	req.Len(queuedStreams.PendingWritesQueue, 1)
	pending := queuedStreams.PendingWritesQueue[0]
	req.Nil(pending.ID)
	req.Equal(reward19.PendingWriteOpSetWeightRecords, pending.Op)
	req.Equal(parentTS.Height()+f.timelock, pending.EffectiveEpoch)

	// Null rounds over the effective epoch, so the write waits for the first award after them.
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(pending.EffectiveEpoch-4))
	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	req.Lessf(head.Height(), pending.EffectiveEpoch,
		"chain head %d has reached the write's effective epoch %d, too late to leave it null",
		head.Height(), pending.EffectiveEpoch)
	for _, blockMiner := range f.blockMiners {
		blockMiner.InjectNulls(pending.EffectiveEpoch + 1 - head.Height())
	}
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(pending.EffectiveEpoch+10))
	lastBeforeDue := tipsetAtOrBefore(f.ctx, t, f.client, pending.EffectiveEpoch)
	req.Lessf(lastBeforeDue.Height(), pending.EffectiveEpoch,
		"effective epoch %d must be a null round", pending.EffectiveEpoch)

	dueAwardTS := kit.TipsetAtOrAfter(f.ctx, t, f.client, pending.EffectiveEpoch)
	req.Greaterf(dueAwardTS.Height(), pending.EffectiveEpoch,
		"the first award after effective epoch %d must follow the null rounds", pending.EffectiveEpoch)
	t.Logf("null rounds span epochs %d to %d, over an effective epoch of %d",
		lastBeforeDue.Height()+1, dueAwardTS.Height()-1, pending.EffectiveEpoch)
	dueActor, dueState, dueStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, dueAwardTS.Key())
	f.requireAllocation(t, dueActor, dueState, dueStreams)
	req.Len(dueStreams.PendingWritesQueue, 1)
	req.Equal(parentStreams.Streams[0].Weight, dueStreams.Streams[0].Weight)
	req.Equal(parentStreams.Streams[1].Weight, dueStreams.Streams[1].Weight)

	appliedTS := kit.TipsetAtOrAfter(f.ctx, t, f.client, dueAwardTS.Height()+1)
	appliedActor, appliedState, appliedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, appliedTS.Key())
	f.requireAllocation(t, appliedActor, appliedState, appliedStreams)
	req.Empty(appliedStreams.PendingWritesQueue)
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

// testQuarterlyGateAndShares covers volume posting, a passing weight step, share submission and
// payout.
func (f *solsticeRewardLifecycle) testQuarterlyGateAndShares(t *testing.T) {
	req := require.New(t)
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(f.quarterStart))
	posted := solsticeInvoke(f.ctx, t, f.client, f.orchestrator, f.sraAddr,
		solsticeSelector(t, "ServiceRewardsActor", "postVolume(uint64,uint256)"),
		kit.EvmWordUint64(f.gateQuarter), kit.EvmWordBytes(f.gateVolumeUSD.Int.Bytes()))
	kit.RequireMessageSuccess(t, posted)
	postedTS, err := f.client.ChainGetTipSet(f.ctx, posted.TipSet)
	req.NoError(err)
	req.Lessf(postedTS.Height(), f.postingEnd, "volume must land inside quarter %d's posting window", f.gateQuarter)

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(f.bindingEpoch))
	gate := solsticeInvoke(f.ctx, t, f.client, f.deployer, f.swaAddr,
		solsticeSelector(t, "StreamWeightActor", "quarterlyGateCheck()"))
	kit.RequireMessageSuccess(t, gate)

	_, _, gateStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, gate.TipSet)
	req.Len(gateStreams.PendingWritesQueue, 1)
	pending := gateStreams.PendingWritesQueue[0]
	req.Nil(pending.ID)
	req.Equal(reward19.PendingWriteOpStepWeightRecords, pending.Op)

	refused := f.cancelPendingWeight(t, f.owner1, reward19.PendingWriteOpStepWeightRecords)
	req.False(refused.Receipt.ExitCode.IsSuccess(), "f02 must refuse to cancel a gate-originated step")
	refusedTS, err := f.client.ChainGetTipSet(f.ctx, refused.TipSet)
	req.NoError(err)
	req.Lessf(refusedTS.Height(), pending.EffectiveEpoch,
		"the cancellation must reach f02 before the step applies at epoch %d", pending.EffectiveEpoch)
	_, _, refusedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, refused.TipSet)
	req.Equal([]reward19.PendingWrite{pending}, refusedStreams.PendingWritesQueue,
		"the refused cancellation leaves the queued step exactly as it was")

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(pending.EffectiveEpoch+2))
	dueAwardTS := kit.TipsetAtOrAfter(f.ctx, t, f.client, pending.EffectiveEpoch)
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(dueAwardTS.Height()+1))
	appliedTS := kit.TipsetAtOrAfter(f.ctx, t, f.client, dueAwardTS.Height()+1)
	appliedActor, appliedState, appliedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, appliedTS.Key())
	f.requireAllocation(t, appliedActor, appliedState, appliedStreams)
	req.Empty(appliedStreams.PendingWritesQueue)
	req.Equal(f.steppedWeight, streamByID(t, appliedStreams, 2).Weight)

	// Anyone may call submitShares once the quarter binds; the SRA writes stream 2's share map.
	submitted := solsticeInvoke(f.ctx, t, f.client, f.deployer, f.sraAddr,
		solsticeSelector(t, "ServiceRewardsActor", "submitShares(uint64)"),
		kit.EvmWordUint64(f.gateQuarter))
	kit.RequireMessageSuccess(t, submitted)
	submittedTS, err := f.client.ChainGetTipSet(f.ctx, submitted.TipSet)
	req.NoError(err)
	_, _, nextBinds := f.quarterEpochs(f.gateQuarter + 1)
	req.Lessf(submittedTS.Height(), nextBinds,
		"quarter %d's shares must be submitted before the next quarter binds", f.gateQuarter)
	submittedActor, submittedState, submittedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, submitted.TipSet)
	f.requireAllocation(t, submittedActor, submittedState, submittedStreams)
	distribution := streamByID(t, submittedStreams, 2).Distribution
	req.Equal([]reward19.RecipientShare{{Recipient: f.orchestratorID, Share: reward19.Denom}}, distribution.Shares)
	req.Len(distribution.Payable, 1)
	req.Equal(f.orchestratorID, distribution.Payable[0].Recipient)
	settledPayable := distribution.Payable[0].Amount
	req.Positive(settledPayable.Sign())

	claim := f.claim(t, 2, []address.Address{f.orchestratorID})
	req.Len(claim.amounts, 1)
	req.Positive(big.Cmp(claim.amounts[0], settledPayable),
		"the claim carries the settled balance plus the current period's accrual")
	claimed := streamByID(t, claim.streams, 2).Distribution
	req.Empty(claimed.Payable)
	req.Len(claimed.ClaimedPeriod, 1)
	req.Equal(f.orchestratorID, claimed.ClaimedPeriod[0].Recipient)
	req.Equal(f.steppedWeight, streamByID(t, claim.streams, 2).Weight)
	f.requireServiceShareInForce(t, appliedState, claim.state, 15*f.pct)
}

// quarterEpochs returns a quarter's posting and binding epochs from the SRA's activation anchor.
func (f *solsticeRewardLifecycle) quarterEpochs(quarter uint64) (opens, postingEnds, binds abi.ChainEpoch) {
	offset := (abi.ChainEpoch(quarter) - abi.ChainEpoch(f.gateQuarter)) * f.epochsPerQuarter
	return f.quarterStart + offset, f.postingEnd + offset, f.bindingEpoch + offset
}

// testFailedGate checks that a below-threshold volume holds stream 2's weight but still submits
// shares.
func (f *solsticeRewardLifecycle) testFailedGate(t *testing.T) {
	req := require.New(t)
	quarter := f.gateQuarter + 1
	opens, postingEnds, binds := f.quarterEpochs(quarter)
	_, _, nextBinds := f.quarterEpochs(quarter + 1)

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(opens))
	posted := solsticeInvoke(f.ctx, t, f.client, f.orchestrator, f.sraAddr,
		solsticeSelector(t, "ServiceRewardsActor", "postVolume(uint64,uint256)"),
		kit.EvmWordUint64(quarter), kit.EvmWordBytes(f.belowThresholdUSD.Int.Bytes()))
	kit.RequireMessageSuccess(t, posted)
	postedTS, err := f.client.ChainGetTipSet(f.ctx, posted.TipSet)
	req.NoError(err)
	req.Lessf(postedTS.Height(), postingEnds, "volume must land inside quarter %d's posting window", quarter)

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(binds))
	beforeTS, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	_, beforeState, beforeStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, beforeTS.Key())
	req.Empty(beforeStreams.PendingWritesQueue)
	req.Equal(f.steppedWeight, streamByID(t, beforeStreams, 2).Weight)

	gate := solsticeInvoke(f.ctx, t, f.client, f.deployer, f.swaAddr,
		solsticeSelector(t, "StreamWeightActor", "quarterlyGateCheck()"))
	kit.RequireMessageSuccess(t, gate)
	gateActor, gateState, gateStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, gate.TipSet)
	f.requireAllocation(t, gateActor, gateState, gateStreams)
	req.Empty(gateStreams.PendingWritesQueue, "a volume below the threshold queues no weight step")
	req.Equal(f.steppedWeight, streamByID(t, gateStreams, 2).Weight)
	req.Len(streamByID(t, gateStreams, 2).Distribution.ClaimedPeriod, 1,
		"the passing quarter's claim is still the open period's only withdrawal")

	submitted := solsticeInvoke(f.ctx, t, f.client, f.deployer, f.sraAddr,
		solsticeSelector(t, "ServiceRewardsActor", "submitShares(uint64)"),
		kit.EvmWordUint64(quarter))
	kit.RequireMessageSuccess(t, submitted)
	submittedTS, err := f.client.ChainGetTipSet(f.ctx, submitted.TipSet)
	req.NoError(err)
	req.Lessf(submittedTS.Height(), nextBinds,
		"quarter %d's shares must be submitted before the next quarter binds", quarter)
	submittedActor, submittedState, submittedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, submitted.TipSet)
	f.requireAllocation(t, submittedActor, submittedState, submittedStreams)
	req.Empty(submittedStreams.PendingWritesQueue)
	req.Equal(f.steppedWeight, streamByID(t, submittedStreams, 2).Weight)
	// A failed gate leaves the volumes bound, so the map still reaches f02 and closes the period.
	distribution := streamByID(t, submittedStreams, 2).Distribution
	req.Equal([]reward19.RecipientShare{{Recipient: f.orchestratorID, Share: reward19.Denom}}, distribution.Shares)
	req.Empty(distribution.ClaimedPeriod, "the closed period carries no withdrawals into the new one")
	req.Len(distribution.Payable, 1)
	req.Equal(f.orchestratorID, distribution.Payable[0].Recipient)
	req.Positive(distribution.Payable[0].Amount.Sign())
	f.requireServiceShareInForce(t, beforeState, submittedState, 15*f.pct)
}

// testMissedVolumePost checks that an unposted quarter advances submission without touching f02.
func (f *solsticeRewardLifecycle) testMissedVolumePost(t *testing.T) {
	req := require.New(t)
	quarter := f.gateQuarter + 2
	_, _, binds := f.quarterEpochs(quarter)
	_, _, nextBinds := f.quarterEpochs(quarter + 1)

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(binds))
	beforeTS, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	_, beforeState, beforeStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, beforeTS.Key())
	req.Empty(beforeStreams.PendingWritesQueue)
	req.Equal(f.steppedWeight, streamByID(t, beforeStreams, 2).Weight)

	gate := solsticeInvoke(f.ctx, t, f.client, f.deployer, f.swaAddr,
		solsticeSelector(t, "StreamWeightActor", "quarterlyGateCheck()"))
	kit.RequireMessageSuccess(t, gate)
	gateActor, gateState, gateStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, gate.TipSet)
	f.requireAllocation(t, gateActor, gateState, gateStreams)
	req.Empty(gateStreams.PendingWritesQueue, "an unposted quarter queues no weight step")
	req.Equal(f.steppedWeight, streamByID(t, gateStreams, 2).Weight)
	beforeDistribution := streamByID(t, gateStreams, 2).Distribution
	beforeAccrual := accrualOf(t, gateState, 2)

	submitted := solsticeInvoke(f.ctx, t, f.client, f.deployer, f.sraAddr,
		solsticeSelector(t, "ServiceRewardsActor", "submitShares(uint64)"),
		kit.EvmWordUint64(quarter))
	kit.RequireMessageSuccess(t, submitted)
	submittedTS, err := f.client.ChainGetTipSet(f.ctx, submitted.TipSet)
	req.NoError(err)
	req.Lessf(submittedTS.Height(), nextBinds,
		"quarter %d's shares must be submitted before the next quarter binds", quarter)
	submittedActor, submittedState, submittedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, submitted.TipSet)
	f.requireAllocation(t, submittedActor, submittedState, submittedStreams)
	req.Equal(f.steppedWeight, streamByID(t, submittedStreams, 2).Weight)
	f.requireServiceShareInForce(t, beforeState, submittedState, 15*f.pct)
	req.Equal(beforeDistribution, streamByID(t, submittedStreams, 2).Distribution,
		"a quarter with no volume leaves the share map, payables and withdrawals untouched")
	req.Positive(big.Cmp(accrualOf(t, submittedState, 2), beforeAccrual),
		"awards keep accruing to the stream across the quarter")

	resubmitted := solsticeInvoke(f.ctx, t, f.client, f.deployer, f.sraAddr,
		solsticeSelector(t, "ServiceRewardsActor", "submitShares(uint64)"),
		kit.EvmWordUint64(quarter))
	req.False(resubmitted.Receipt.ExitCode.IsSuccess(), "a quarter cannot be submitted twice")
}

// testSecondOrchestrator covers a two-row quarter: shares proportional to the posted volumes, a
// removal that burns the departing slice, a prospective wallet swap, and the next redistribution.
func (f *solsticeRewardLifecycle) testSecondOrchestrator(t *testing.T) {
	req := require.New(t)
	quarter := f.gateQuarter + 3
	opens, postingEnds, binds := f.quarterEpochs(quarter)
	nextOpens, nextPostingEnds, nextBinds := f.quarterEpochs(quarter + 1)

	isAdmitted := solsticeSelector(t, "ServiceRewardsActor", "isAdmitted(address)")
	orchestratorCount := solsticeSelector(t, "ServiceRewardsActor", "orchestratorCount()")
	postVolume := solsticeSelector(t, "ServiceRewardsActor", "postVolume(uint64,uint256)")
	submitShares := solsticeSelector(t, "ServiceRewardsActor", "submitShares(uint64)")
	orchestrator1Word := kit.EvmWordFromAddr(f.ctx, t, f.client, f.orchestrator)
	orchestrator2Word := kit.EvmWordFromAddr(f.ctx, t, f.client, f.orchestrator2)
	wallet1BWord := kit.EvmWordFromAddr(f.ctx, t, f.client, f.wallet1B)
	usd := func(amount int64) []byte {
		return kit.EvmWordBytes(big.Mul(big.NewInt(amount), big.NewInt(1_000_000_000_000_000_000)).Int.Bytes())
	}
	sraRead := func(selector []byte, args ...[]byte) *api.MsgLookup {
		return solsticeInvoke(f.ctx, t, f.client, f.deployer, f.sraAddr, selector, args...)
	}

	// The second orchestrator is its own payout wallet.
	f.requireUnanimous(t, f.sraAddr,
		solsticeSelector(t, "ServiceRewardsActor", "addOrchestrator(address,address)"),
		orchestrator2Word, orchestrator2Word)
	req.True(solsticeBool(t, sraRead(isAdmitted, orchestrator2Word)), "the admitted orchestrator must be seated")
	req.Equal(kit.EvmWordUint64(2), solsticeWord(t, sraRead(orchestratorCount)))

	// Volumes in a 4:1 ratio, so the quarter's share map is 80% and 20%.
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(opens))
	for _, post := range []struct {
		from   address.Address
		volume int64
	}{{f.orchestrator, 4000}, {f.orchestrator2, 1000}} {
		posted := solsticeInvoke(f.ctx, t, f.client, post.from, f.sraAddr, postVolume,
			kit.EvmWordUint64(quarter), usd(post.volume))
		kit.RequireMessageSuccess(t, posted)
		postedTS, err := f.client.ChainGetTipSet(f.ctx, posted.TipSet)
		req.NoError(err)
		req.Lessf(postedTS.Height(), postingEnds, "volume must land inside quarter %d's posting window", quarter)
	}

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(binds))
	submitted := solsticeInvoke(f.ctx, t, f.client, f.deployer, f.sraAddr, submitShares, kit.EvmWordUint64(quarter))
	kit.RequireMessageSuccess(t, submitted)
	submittedActor, submittedState, submittedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, submitted.TipSet)
	f.requireAllocation(t, submittedActor, submittedState, submittedStreams)
	shares := streamByID(t, submittedStreams, 2).Distribution.Shares
	req.Len(shares, 2)
	req.Equal(80*f.pct, shareOf(shares, f.orchestratorID), "shares follow the posted volumes")
	req.Equal(20*f.pct, shareOf(shares, f.orchestrator2ID), "shares follow the posted volumes")

	// One claim for both wallets divides the same pool, so their withdrawals hold the same ratio.
	claim := f.claim(t, 2, []address.Address{f.orchestratorID, f.orchestrator2ID})
	req.Positive(claim.amounts[0].Sign())
	req.Positive(claim.amounts[1].Sign())
	claimed := streamByID(t, claim.streams, 2).Distribution.ClaimedPeriod
	liveFirst := payableOf(claimed, f.orchestratorID)
	liveSecond := payableOf(claimed, f.orchestrator2ID)
	requireShareWithinAtto(t, liveSecond, big.Add(liveFirst, liveSecond), 20*f.pct, 1)

	// The SRA refuses a removal while an ended quarter awaits its share map, so remove before this one ends.
	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	req.Lessf(head.Height(), nextOpens,
		"quarter %d ends at epoch %d, where the removal guard closes again", quarter, nextOpens)
	t.Logf("removing at epoch %d, %d epochs before quarter %d ends", head.Height(), nextOpens-head.Height(), quarter)
	removed := f.requireUnanimous(t, f.sraAddr,
		solsticeSelector(t, "ServiceRewardsActor", "removeOrchestrator(address)"), orchestrator2Word)
	removedTS, err := f.client.ChainGetTipSet(f.ctx, removed.TipSet)
	req.NoError(err)
	removedActor, removedState, removedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, removed.TipSet)
	f.requireAllocation(t, removedActor, removedState, removedStreams)
	removedDistribution := streamByID(t, removedStreams, 2).Distribution
	req.Len(removedDistribution.Shares, 1)
	req.Equal(80*f.pct, shareOf(removedDistribution.Shares, f.orchestratorID),
		"the survivor keeps its own share until the next submission")
	secondCarried := payableOf(removedDistribution.Payable, f.orchestrator2ID)
	req.Positive(secondCarried.Sign(), "the removed wallet keeps what it earned")
	req.False(solsticeBool(t, sraRead(isAdmitted, orchestrator2Word)), "the removed orchestrator must be gone")
	req.Equal(kit.EvmWordUint64(1), solsticeWord(t, sraRead(orchestratorCount)))

	// The dropped 20% of stream 2's 15% weight burns until the map changes again.
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(removedTS.Height()+9))
	_, burnStart, _ := f.stateAtOrAfter(t, removedTS.Height())
	_, burnEnd, _ := f.stateAtOrAfter(t, removedTS.Height()+8)
	f.requireServiceShareInForce(t, burnStart, burnEnd, 12*f.pct)
	burnDelta := rewardDeltas(burnStart, burnEnd)
	requireShareWithinAtto(t, burnDelta.burn, burnDelta.total, 18*f.pct, 2*int64(burnEnd.Epoch-burnStart.Epoch))

	// The wallet swap renames the row and leaves earned balances where they are.
	replaced := f.requireUnanimous(t, f.sraAddr,
		solsticeSelector(t, "ServiceRewardsActor", "replaceWallet(address,address)"),
		orchestrator1Word, wallet1BWord)
	replacedActor, replacedState, replacedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, replaced.TipSet)
	f.requireAllocation(t, replacedActor, replacedState, replacedStreams)
	replacedDistribution := streamByID(t, replacedStreams, 2).Distribution
	req.Len(replacedDistribution.Shares, 1)
	req.Equal(80*f.pct, shareOf(replacedDistribution.Shares, f.wallet1BID), "the share moves to the new wallet")
	req.Zero(shareOf(replacedDistribution.Shares, f.orchestratorID), "the old wallet earns nothing from here on")
	firstCarried := payableOf(replacedDistribution.Payable, f.orchestratorID)
	req.Positive(firstCarried.Sign(), "the old wallet keeps what it earned")
	req.Zero(payableOf(replacedDistribution.Payable, f.wallet1BID).Sign(), "the new wallet starts from zero")
	requireAddressReplacedEvents(f.ctx, t, f.client, f.messageHeight(t, replaced), 2, f.orchestratorID, f.wallet1BID)

	payouts := f.claim(t, 2, []address.Address{f.orchestratorID, f.orchestrator2ID, f.wallet1BID})
	req.Equalf(0, big.Cmp(firstCarried, payouts.amounts[0]),
		"the old wallet is paid the %s it carried, not %s", firstCarried, payouts.amounts[0])
	req.Equalf(0, big.Cmp(secondCarried, payouts.amounts[1]),
		"the removed wallet is paid the %s it carried, not %s", secondCarried, payouts.amounts[1])
	req.Positive(payouts.amounts[2].Sign(), "the new wallet is paid its live share while the map sums below Denom")

	// The next submission redistributes to the survivor alone, under the wallet it now uses.
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(nextOpens))
	nextPosted := solsticeInvoke(f.ctx, t, f.client, f.orchestrator, f.sraAddr, postVolume,
		kit.EvmWordUint64(quarter+1), usd(4000))
	kit.RequireMessageSuccess(t, nextPosted)
	nextPostedTS, err := f.client.ChainGetTipSet(f.ctx, nextPosted.TipSet)
	req.NoError(err)
	req.Lessf(nextPostedTS.Height(), nextPostingEnds,
		"volume must land inside quarter %d's posting window", quarter+1)

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(nextBinds))
	resubmitted := solsticeInvoke(f.ctx, t, f.client, f.deployer, f.sraAddr, submitShares, kit.EvmWordUint64(quarter+1))
	kit.RequireMessageSuccess(t, resubmitted)
	resubmittedTS, err := f.client.ChainGetTipSet(f.ctx, resubmitted.TipSet)
	req.NoError(err)
	finalActor, finalState, finalStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, resubmitted.TipSet)
	f.requireAllocation(t, finalActor, finalState, finalStreams)
	finalDistribution := streamByID(t, finalStreams, 2).Distribution
	req.Equal([]reward19.RecipientShare{{Recipient: f.wallet1BID, Share: reward19.Denom}},
		finalDistribution.Shares, "the survivor takes the whole map under its new wallet")
	req.Positive(payableOf(finalDistribution.Payable, f.wallet1BID).Sign(),
		"the new wallet earns from the replacement onward")
	req.Zero(payableOf(finalDistribution.Payable, f.orchestratorID).Sign(),
		"the old wallet's balance left with its claim")

	replacementPayout := f.claim(t, 2, []address.Address{f.wallet1BID})
	req.Positive(replacementPayout.amounts[0].Sign(), "the new wallet claims what it has earned")

	// A map that sums to Denom again leaves stream 2's weight as the only explicit share.
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(resubmittedTS.Height()+9))
	_, wholeStart, _ := f.stateAtOrAfter(t, resubmittedTS.Height())
	_, wholeEnd, _ := f.stateAtOrAfter(t, resubmittedTS.Height()+8)
	f.requireServiceShareInForce(t, wholeStart, wholeEnd, 15*f.pct)
	wholeDelta := rewardDeltas(wholeStart, wholeEnd)
	requireShareWithinAtto(t, wholeDelta.burn, wholeDelta.total, 15*f.pct, 2*int64(wholeEnd.Epoch-wholeStart.Epoch))
}

func (f *solsticeRewardLifecycle) testStreamRegistration(t *testing.T) {
	req := require.New(t)
	// Registration requires activation >= execution epoch + timelock; the slack covers inclusion
	// delay.
	const registerSlack = abi.ChainEpoch(20)
	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	activation := head.Height() + f.timelock + registerSlack
	weight := flatWeight(5 * f.pct)
	shares := []reward19.RecipientShare{{Recipient: f.recipients[0], Share: reward19.Denom}}

	lookup := f.requireUnanimous(t, f.swaAddr,
		solsticeSelector(t, "StreamWeightActor",
			"registerStream(uint64,(int256,int256,uint64,int256,int256),address,(address,uint256)[],uint64)"),
		solsticeRegisterStreamInput(f.ctx, t, f.client, f.newStream, weight, f.w3WriterID, shares, activation))

	queuedActor, queuedState, queuedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, lookup.TipSet)
	req.Len(queuedStreams.PendingWritesQueue, 1)
	pending := queuedStreams.PendingWritesQueue[0]
	req.NotNil(pending.ID)
	req.Equal(f.newStream, *pending.ID)
	req.Equal(reward19.PendingWriteOpRegisterStream, pending.Op)
	req.Equal(activation, pending.EffectiveEpoch)
	req.Len(queuedStreams.Streams, 2, "a queued registration leaves the stream table alone")

	ledger := f.requireAdapterLedger(t, lookup.TipSet, queuedActor, queuedState, queuedStreams)
	queuedRegistration := ledger.PendingWrites[0].Register
	req.NotNil(queuedRegistration)
	req.Equal(weight, queuedRegistration.Weight)
	req.Equal(f.w3WriterID, queuedRegistration.Distribution.Writer)
	req.Equal(shares, queuedRegistration.Distribution.Shares)

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(activation+2))
	dueAwardTS := kit.TipsetAtOrAfter(f.ctx, t, f.client, activation)
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(dueAwardTS.Height()+1))
	liveActor, liveState, liveStreams := f.stateAtOrAfter(t, dueAwardTS.Height()+1)
	f.requireAllocation(t, liveActor, liveState, liveStreams)
	req.Empty(liveStreams.PendingWritesQueue)
	req.Len(liveStreams.Streams, 3)
	registered := streamByID(t, liveStreams, f.newStream)
	req.Equal(weight, registered.Weight)
	req.Equal(f.w3WriterID, registered.Distribution.Writer)
	req.Equal(shares, registered.Distribution.Shares)
	req.Positive(accrualOf(t, liveState, f.newStream).Sign())

	requireRegistrationEventVisibility(f.ctx, t, f.client, f.messageHeight(t, lookup), dueAwardTS.Height(), f.newStream)
}

// testShareSettlementAndWalletPayouts exercises share replacement, partial claims, and recipient balance changes.
func (f *solsticeRewardLifecycle) testShareSettlementAndWalletPayouts(t *testing.T) {
	req := require.New(t)
	newShares := []reward19.RecipientShare{
		{Recipient: f.recipients[0], Share: 40 * f.pct},
		{Recipient: f.recipients[1], Share: 60 * f.pct},
	}
	lookup := f.sendRewardMessage(t, f.w3WriterKey.Address, builtin.MethodsReward.SetSharesExported, &reward19.SetSharesParams{
		ID:     f.newStream,
		Shares: newShares,
	})
	kit.RequireMessageSuccess(t, lookup)
	accrued, dust := f.requireFoldDustBurned(t, lookup, f.newStream)
	requireSetSharesEvents(f.ctx, t, f.client, f.messageHeight(t, lookup), f.newStream, accrued, dust, newShares)

	settledActor, settledState, settledStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, lookup.TipSet)
	f.requireAllocation(t, settledActor, settledState, settledStreams)
	distribution := streamByID(t, settledStreams, f.newStream).Distribution
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
	awardTS, err := f.client.ChainGetTipSet(f.ctx, setSharesTS.Parents())
	req.NoError(err)
	_, _, previousStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, awardTS.Key())
	req.Len(streamByID(t, previousStreams, f.newStream).Distribution.Shares, 1,
		"the block starts with the previous one-recipient share map")
	reported, err := f.client.V2.StateRewardDistribution(f.ctx, types.TipSetSelectors.Key(awardTS.Key()))
	req.NoError(err)
	requireRewardAPIConservation(t, reported)
	req.Len(reported.Blocks, 1)
	var reportedStream *v2api.StreamReward
	for i := range reported.Blocks[0].Streams {
		stream := &reported.Blocks[0].Streams[i]
		if stream.ID == uint64(f.newStream) {
			reportedStream = stream
			break
		}
	}
	req.NotNil(reportedStream)
	req.NotNil(reportedStream.Distribution)
	req.Len(reportedStream.Distribution.Recipients, 2,
		"the award must use SetShares from its own block, not the parent-state map")
	for i, recipient := range reportedStream.Distribution.Recipients {
		req.Equal(distribution.Shares[i].Recipient, recipient.Recipient)
		req.Equal(distribution.Shares[i].Share, recipient.Share)
		// SetShares resets the period before this award; prior-period payable
		// balances must not be included in the block's recipient earnings.
		req.Equal(accruedShare(accrualOf(t, settledState, f.newStream), recipient.Share), recipient.EarnedAmount)
	}
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(setSharesTS.Height()+5))
	beforeClaimsTS := kit.TipsetAtOrAfter(f.ctx, t, f.client, setSharesTS.Height()+5)
	beforeBalances := []abi.TokenAmount{
		kit.MustActor(f.ctx, t, f.client, f.recipients[0], beforeClaimsTS.Key()).Balance,
		kit.MustActor(f.ctx, t, f.client, f.recipients[1], beforeClaimsTS.Key()).Balance,
	}

	first := f.claim(t, f.newStream, f.recipients[:1])
	firstDistribution := streamByID(t, first.streams, f.newStream).Distribution
	req.Empty(firstDistribution.Payable)
	req.Len(firstDistribution.ClaimedPeriod, 1)
	req.Equal(f.recipients[0], firstDistribution.ClaimedPeriod[0].Recipient)
	firstCurrentClaim := firstDistribution.ClaimedPeriod[0].Amount
	req.Positive(firstCurrentClaim.Sign())
	req.Equal(big.Add(settledPayable, firstCurrentClaim), first.amounts[0])
	pendingRecipient2 := accruedShare(accrualOf(t, first.state, f.newStream), 60*f.pct)
	req.Positive(pendingRecipient2.Sign())

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(first.tipset.Height()+5))
	second := f.claim(t, f.newStream, f.recipients)
	secondDistribution := streamByID(t, second.streams, f.newStream).Distribution
	req.Empty(secondDistribution.Payable)
	req.Len(secondDistribution.ClaimedPeriod, 2)
	req.Equal(f.recipients[0], secondDistribution.ClaimedPeriod[0].Recipient)
	req.Equal(f.recipients[1], secondDistribution.ClaimedPeriod[1].Recipient)

	secondAccrual := accrualOf(t, second.state, f.newStream)
	currentRecipient1 := accruedShare(secondAccrual, 40*f.pct)
	currentRecipient2 := accruedShare(secondAccrual, 60*f.pct)
	req.Positive(big.Cmp(currentRecipient1, secondDistribution.ClaimedPeriod[0].Amount))
	req.Positive(big.Cmp(currentRecipient2, secondDistribution.ClaimedPeriod[1].Amount))
	req.Equal(big.Sub(secondDistribution.ClaimedPeriod[0].Amount, firstCurrentClaim), second.amounts[0])
	req.Equal(secondDistribution.ClaimedPeriod[1].Amount, second.amounts[1])
	req.Positive(big.Cmp(second.amounts[1], pendingRecipient2))

	cumulativePaid := []abi.TokenAmount{big.Add(first.amounts[0], second.amounts[0]), second.amounts[1]}
	req.Equal(big.Add(settledPayable, secondDistribution.ClaimedPeriod[0].Amount), cumulativePaid[0])
	req.Equal(secondDistribution.ClaimedPeriod[1].Amount, cumulativePaid[1])
	for i, recipient := range f.recipients {
		after := kit.MustActor(f.ctx, t, f.client, recipient, second.tipset.Key()).Balance
		req.Equal(cumulativePaid[i], big.Sub(after, beforeBalances[i]))
	}

	currentPeriodTotal := big.Add(secondDistribution.ClaimedPeriod[0].Amount, secondDistribution.ClaimedPeriod[1].Amount)
	requireShareWithinAtto(t, secondDistribution.ClaimedPeriod[0].Amount, currentPeriodTotal, 40*f.pct, 1)
	requireShareWithinAtto(t, secondDistribution.ClaimedPeriod[1].Amount, currentPeriodTotal, 60*f.pct, 1)
}

// testRemoveStreamTombstoneClaim exercises stream removal: queued, tombstoned, claimed, deleted.
func (f *solsticeRewardLifecycle) testRemoveStreamTombstoneClaim(t *testing.T) {
	req := require.New(t)
	setShares := f.sendRewardMessage(t, f.w3WriterKey.Address, builtin.MethodsReward.SetSharesExported, &reward19.SetSharesParams{
		ID: f.newStream,
		Shares: []reward19.RecipientShare{
			{Recipient: f.recipients[0], Share: 40 * f.pct},
			{Recipient: f.recipients[1], Share: 60 * f.pct},
		},
	})
	kit.RequireMessageSuccess(t, setShares)
	// A two-row fold is where dust actually appears.
	f.requireFoldDustBurned(t, setShares, f.newStream)

	lookup := f.requireUnanimous(t, f.swaAddr,
		solsticeSelector(t, "StreamWeightActor", "removeStream(uint64)"),
		kit.EvmWordUint64(uint64(f.newStream)))
	queuedTS, err := f.client.ChainGetTipSet(f.ctx, lookup.TipSet)
	req.NoError(err)
	_, _, queuedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, lookup.TipSet)
	req.Len(queuedStreams.PendingWritesQueue, 1)
	pending := queuedStreams.PendingWritesQueue[0]
	req.NotNil(pending.ID)
	req.Equal(f.newStream, *pending.ID)
	req.Equal(reward19.PendingWriteOpRemoveStream, pending.Op)

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(pending.EffectiveEpoch+2))
	dueTS := kit.TipsetAtOrAfter(f.ctx, t, f.client, pending.EffectiveEpoch)
	appliedTS := kit.TipsetAtOrAfter(f.ctx, t, f.client, dueTS.Height()+1)
	removedActor, removed, removedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, appliedTS.Key())
	f.requireAllocation(t, removedActor, removed, removedStreams)
	f.requireAdapterLedger(t, appliedTS.Key(), removedActor, removed, removedStreams)
	req.Len(removedStreams.Streams, 2)
	req.Equal(reward19.StreamID(1), removedStreams.Streams[0].ID)
	req.Equal(reward19.StreamID(2), removedStreams.Streams[1].ID)
	req.Len(removedStreams.Tombstones, 1)
	req.Equal(f.newStream, removedStreams.Tombstones[0].ID)
	req.Len(removedStreams.Tombstones[0].Payable, 2)
	payable := append([]reward19.RecipientAmount(nil), removedStreams.Tombstones[0].Payable...)

	// Stream 1 holds 70% from the deferred write and stream 2 15% from the gate, so while w3 waits
	// for removal the explicit share is 20% and the burn 10%. Each stream rounds its award down, an
	// atto per stream per epoch.
	_, liveStart, _ := f.stateAtOrAfter(t, queuedTS.Height())
	_, liveEnd, _ := f.stateAtOrAfter(t, pending.EffectiveEpoch)
	liveEpochs := int64(liveEnd.Epoch - liveStart.Epoch)
	req.Positive(liveEpochs)
	liveDelta := rewardDeltas(liveStart, liveEnd)
	requireShareWithinAtto(t, liveDelta.service, liveDelta.total, 20*f.pct, 2*liveEpochs)
	requireShareWithinAtto(t, liveDelta.burn, liveDelta.total, 10*f.pct, 3*liveEpochs)

	// Once the removal applies, w2's 15% is the whole explicit share and w3's 5% joins the burn.
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(appliedTS.Height()+6))
	_, goneStart, _ := f.stateAtOrAfter(t, appliedTS.Height()+1)
	_, goneEnd, _ := f.stateAtOrAfter(t, appliedTS.Height()+6)
	f.requireServiceShareInForce(t, goneStart, goneEnd, 15*f.pct)
	goneDelta := rewardDeltas(goneStart, goneEnd)
	requireShareWithinAtto(t, goneDelta.burn, goneDelta.total, 15*f.pct, 2*int64(goneEnd.Epoch-goneStart.Epoch))

	claim := f.claim(t, f.newStream, f.recipients)
	req.Len(claim.amounts, len(payable))
	for i, row := range payable {
		req.Equal(f.recipients[i], row.Recipient)
		req.Equal(row.Amount, claim.amounts[i])
		req.Equal(row.Amount, claim.deltas[i])
		req.Positive(claim.amounts[i].Sign())
	}
	req.Empty(claim.streams.Tombstones)
	req.Len(claim.streams.Streams, 2)

	// A claim on a stream that is neither live nor tombstoned returns zero per wallet.
	drained := f.claim(t, f.newStream, f.recipients)
	for i, amount := range drained.amounts {
		req.Zerof(amount.Sign(), "claiming the drained tombstone must pay recipient %d nothing", i)
	}
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

// unanimousInvoke pushes both owners' approvals while mining is paused and returns both receipts.
// Either owner's approval may execute first, so callers read state at the later receipt.
func (f *solsticeRewardLifecycle) unanimousInvoke(t *testing.T, to address.Address, selector []byte, args ...[]byte) (*api.MsgLookup, *api.MsgLookup) {
	t.Helper()
	var first, second cid.Cid
	func() {
		for _, blockMiner := range f.blockMiners {
			blockMiner.Pause()
		}
		// A push that fails ends the subtest, and a chain left paused hangs every test after it.
		defer func() {
			for _, blockMiner := range f.blockMiners {
				blockMiner.Restart()
			}
		}()
		first = solsticePush(f.ctx, t, f.client, f.owner1, to, selector, args...)
		second = solsticePush(f.ctx, t, f.client, f.owner2, to, selector, args...)
	}()
	return solsticeWait(f.ctx, t, f.client, first), solsticeWait(f.ctx, t, f.client, second)
}

// requireUnanimous requires both approvals to succeed and returns the later receipt.
func (f *solsticeRewardLifecycle) requireUnanimous(t *testing.T, to address.Address, selector []byte, args ...[]byte) *api.MsgLookup {
	t.Helper()
	first, second := f.unanimousInvoke(t, to, selector, args...)
	kit.RequireMessageSuccess(t, first)
	kit.RequireMessageSuccess(t, second)
	return solsticeLaterLookup(first, second)
}

func (f *solsticeRewardLifecycle) swaSetWeightRecords(t *testing.T, updates []reward19.WeightRecordUpdate) *api.MsgLookup {
	t.Helper()
	return f.requireUnanimous(t, f.swaAddr, solsticeSetWeightRecords(t), solsticeWeightUpdatesInput(updates))
}

// cancelPendingWeight clears a weight slot, which any single owner may do.
func (f *solsticeRewardLifecycle) cancelPendingWeight(t *testing.T, from address.Address, op reward19.PendingWriteOp) *api.MsgLookup {
	t.Helper()
	return solsticeInvoke(f.ctx, t, f.client, from, f.swaAddr,
		solsticeSelector(t, "StreamWeightActor", "cancelPendingWeight(uint8)"),
		kit.EvmWordUint64(uint64(op)))
}

func (f *solsticeRewardLifecycle) claim(t *testing.T, id reward19.StreamID, wallets []address.Address) solsticeClaimResult {
	t.Helper()
	req := require.New(t)
	lookup := f.sendRewardMessage(t, f.miner.OwnerKey.Address, builtin.MethodsReward.ClaimExported, &reward19.ClaimParams{ID: id, Wallets: wallets})
	kit.RequireMessageSuccess(t, lookup)
	var result reward19.ClaimReturn
	req.NoError(result.UnmarshalCBOR(bytes.NewReader(lookup.Receipt.Return)))
	req.Len(result.Amounts, len(wallets))

	claimTS, err := f.client.ChainGetTipSet(f.ctx, lookup.TipSet)
	req.NoError(err)
	beforeTS, err := f.client.ChainGetTipSet(f.ctx, claimTS.Parents())
	req.NoError(err)
	deltas := make([]abi.TokenAmount, len(wallets))
	for i, recipient := range wallets {
		before := kit.MustActor(f.ctx, t, f.client, recipient, beforeTS.Key()).Balance
		after := kit.MustActor(f.ctx, t, f.client, recipient, claimTS.Key()).Balance
		deltas[i] = big.Sub(after, before)
		// Equal big.Ints can differ in representation.
		req.Equalf(0, big.Cmp(result.Amounts[i], deltas[i]),
			"wallet %s balance moved by %s, not the claimed %s", recipient, deltas[i], result.Amounts[i])
	}
	actor, state, streams := kit.LoadReward19(f.ctx, t, f.client, f.store, lookup.TipSet)
	f.requireAllocation(t, actor, state, streams)
	return solsticeClaimResult{tipset: claimTS, amounts: result.Amounts, deltas: deltas, state: state, streams: streams}
}

func (f *solsticeRewardLifecycle) stateAtOrAfter(t *testing.T, epoch abi.ChainEpoch) (*types.Actor, *reward19.State, *reward19.StreamsState) {
	t.Helper()
	ts := kit.TipsetAtOrAfter(f.ctx, t, f.client, epoch)
	return kit.LoadReward19(f.ctx, t, f.client, f.store, ts.Key())
}

// messageHeight returns the height of the tipset that executed lookup's message, one epoch behind
// its receipt.
func (f *solsticeRewardLifecycle) messageHeight(t *testing.T, lookup *api.MsgLookup) abi.ChainEpoch {
	t.Helper()
	req := require.New(t)
	receiptTS, err := f.client.ChainGetTipSet(f.ctx, lookup.TipSet)
	req.NoError(err)
	messageTS, err := f.client.ChainGetTipSet(f.ctx, receiptTS.Parents())
	req.NoError(err)
	return messageTS.Height()
}

// requireServiceShareInForce checks the share f02 actually paid the explicit streams, not the
// stored weight. Stream 2 is the only explicit stream in the gate phases.
func (f *solsticeRewardLifecycle) requireServiceShareInForce(t *testing.T, start, end *reward19.State, share uint64) {
	t.Helper()
	delta := rewardDeltas(start, end)
	awardUpperBound := int64(end.Epoch - start.Epoch)
	require.Positive(t, awardUpperBound)
	requireShareWithinAtto(t, delta.service, delta.total, share, awardUpperBound)
}

// requireFoldDustBurned checks the message sends its fold's rounding residue to f099 while the
// tipset's award moves the counters by the weight split alone. Returns the period's accrual and
// the dust from the fold.
func (f *solsticeRewardLifecycle) requireFoldDustBurned(t *testing.T, lookup *api.MsgLookup, id reward19.StreamID) (accrued, dust abi.TokenAmount) {
	t.Helper()
	req := require.New(t)
	receiptTS, err := f.client.ChainGetTipSet(f.ctx, lookup.TipSet)
	req.NoError(err)
	messageTS, err := f.client.ChainGetTipSet(f.ctx, receiptTS.Parents())
	req.NoError(err)
	req.Len(messageTS.Blocks(), 1, "the split below accounts for a single block's award")

	_, before, beforeStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, messageTS.Key())
	_, after, afterStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, receiptTS.Key())
	req.Empty(beforeStreams.PendingWritesQueue, "a due write would fold and burn alongside the message")
	req.Empty(afterStreams.PendingWritesQueue)

	// Messages run before their block's award, so the fold divided the pool the message found.
	accrued = accrualOf(t, before, id)
	dust = foldDust(accrued, streamByID(t, beforeStreams, id).Distribution.Shares)
	replay, err := f.client.StateReplay(f.ctx, types.EmptyTSK, lookup.Message)
	req.NoError(err)
	sent := burntFundsSent(replay.ExecutionTrace)
	req.Equalf(0, big.Cmp(dust, sent), "the fold left %s of dust and the message sent %s to f099", dust, sent)
	t.Logf("stream %d fold left %s attoFIL of dust", id, dust)

	blockReward := big.Sub(after.TotalMintedReward, before.TotalMintedReward)
	explicit, burn := big.Zero(), blockReward
	for _, stream := range afterStreams.Streams {
		req.Zerof(stream.Weight.Slope, "stream %d still slopes, so its award share is not VStart", stream.ID)
		portion := big.Div(big.Mul(blockReward, big.NewIntUnsigned(stream.Weight.VStart)), denomInt())
		burn = big.Sub(burn, portion)
		if stream.Distribution == nil {
			continue
		}
		// A map summing below Denom pays its streams only that fraction and burns the rest.
		if shareTotal := shareTotalOf(stream.Distribution.Shares); big.Cmp(shareTotal, denomInt()) != 0 {
			paid := big.Div(big.Mul(portion, shareTotal), denomInt())
			burn = big.Add(burn, big.Sub(portion, paid))
			portion = paid
		}
		explicit = big.Add(explicit, portion)
	}
	countedBurn := big.Sub(after.TotalBurnMinted, before.TotalBurnMinted)
	countedExplicit := big.Sub(after.TotalExplicitMinted, before.TotalExplicitMinted)
	req.Equalf(0, big.Cmp(burn, countedBurn),
		"the award's weight split burns %s but the burn counter moved %s", burn, countedBurn)
	req.Equalf(0, big.Cmp(explicit, countedExplicit),
		"the award's weight split accrues %s but the explicit counter moved %s", explicit, countedExplicit)

	burntBefore := kit.MustActor(f.ctx, t, f.client, builtin.BurntFundsActorAddr, messageTS.Key()).Balance
	burntAfter := kit.MustActor(f.ctx, t, f.client, builtin.BurntFundsActorAddr, receiptTS.Key()).Balance
	req.GreaterOrEqualf(big.Cmp(big.Sub(burntAfter, burntBefore), big.Add(countedBurn, dust)), 0,
		"f099 must receive the counted burn %s and the dust %s", countedBurn, dust)
	return accrued, dust
}

func (f *solsticeRewardLifecycle) requireAllocation(t *testing.T, actor *types.Actor, state *reward19.State, streams *reward19.StreamsState) {
	t.Helper()
	require.Equal(t, f.initialAllocation, rewardAllocationAt(t, actor, state, streams))
}

func (f *solsticeRewardLifecycle) requireAdapterLedger(
	t *testing.T,
	tsk types.TipSetKey,
	actor *types.Actor,
	state *reward19.State,
	streams *reward19.StreamsState,
) *reward.StreamLedger {
	t.Helper()
	req := require.New(t)
	ts, err := f.client.ChainGetTipSet(f.ctx, tsk)
	req.NoError(err)
	adapted, err := reward.Load(adt.WrapStore(f.ctx, f.store), actor)
	req.NoError(err)
	ledger, err := adapted.StreamLedger(ts.Height())
	req.NoError(err)

	req.Equal(ts.Height(), ledger.Epoch)
	req.Equal(state.SWAActor, ledger.SWAActor)
	req.Equal(state.SWATimelockEpochs, ledger.SWATimelock)
	req.Equal(state.TotalMintedReward, ledger.TotalMinted)
	req.Equal(state.TotalBurnMinted, ledger.TotalBurnMinted)
	req.Equal(state.TotalExplicitMinted, ledger.TotalExplicitMinted)
	liabilities := explicitServiceLiabilities(t, state, streams)
	req.Equalf(0, big.Cmp(liabilities, ledger.Liability()),
		"the adapter holds %s for recipients where the state holds %s", ledger.Liability(), liabilities)

	req.Len(ledger.Streams, len(streams.Streams))
	for i, stream := range streams.Streams {
		read := ledger.Streams[i]
		req.Equal(stream.ID, read.ID)
		req.Equal(stream.Weight, read.Weight)
		req.Equal(stream.Distribution == nil, read.Implicit)
		req.GreaterOrEqual(read.EvaluatedWeight, stream.Weight.Floor)
		req.LessOrEqual(read.EvaluatedWeight, stream.Weight.Cap)
		if stream.Weight.Slope == 0 {
			req.Equal(stream.Weight.VStart, read.EvaluatedWeight, "a flat record holds its weight at every epoch")
		}
		if stream.Distribution == nil {
			continue
		}
		req.Equal(stream.Distribution.Writer, read.Writer)
		req.Equal(stream.Distribution.Shares, read.Shares)
		req.Equal(stream.Distribution.Payable, read.Payable)
		req.Equal(stream.Distribution.ClaimedPeriod, read.ClaimedPeriod)
		req.Equal(accrualOf(t, state, stream.ID), read.Accrued)
	}

	req.Len(ledger.Tombstones, len(streams.Tombstones))
	for i, tombstone := range streams.Tombstones {
		req.Equal(tombstone.ID, ledger.Tombstones[i].ID)
		req.Equal(tombstone.Payable, ledger.Tombstones[i].Payable)
	}

	req.Len(ledger.PendingWrites, len(streams.PendingWritesQueue))
	for i, write := range streams.PendingWritesQueue {
		read := ledger.PendingWrites[i]
		req.Equal(write.ID, read.ID)
		req.EqualValues(write.Op, read.Op)
		req.Equal(write.EffectiveEpoch, read.EffectiveEpoch)
	}

	return ledger
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

func denomInt() big.Int {
	return big.NewIntUnsigned(reward19.Denom)
}

func shareTotalOf(shares []reward19.RecipientShare) big.Int {
	total := big.Zero()
	for _, share := range shares {
		total = big.Add(total, big.NewIntUnsigned(share.Share))
	}
	return total
}

// shareOf returns one recipient's share of a stream, zero when the map has no row for it.
func shareOf(shares []reward19.RecipientShare, recipient address.Address) uint64 {
	for _, share := range shares {
		if share.Recipient == recipient {
			return share.Share
		}
	}
	return 0
}

// payableOf returns one recipient's carried balance, zero when the table has no row for it.
func payableOf(rows []reward19.RecipientAmount, recipient address.Address) abi.TokenAmount {
	for _, row := range rows {
		if row.Recipient == recipient {
			return row.Amount
		}
	}
	return big.Zero()
}

// foldDust is the residue a period fold leaves behind: the pool less each share's floored slice.
func foldDust(pool abi.TokenAmount, shares []reward19.RecipientShare) abi.TokenAmount {
	total := shareTotalOf(shares)
	if total.IsZero() {
		return pool
	}
	allocated := big.Zero()
	for _, share := range shares {
		allocated = big.Add(allocated, big.Div(big.Mul(pool, big.NewIntUnsigned(share.Share)), total))
	}
	return big.Sub(pool, allocated)
}

// burntFundsSent totals what one message's execution sent to f099.
func burntFundsSent(trace types.ExecutionTrace) abi.TokenAmount {
	sent := big.Zero()
	if trace.Msg.To == builtin.BurntFundsActorAddr {
		sent = big.Add(sent, trace.Msg.Value)
	}
	for _, subcall := range trace.Subcalls {
		sent = big.Add(sent, burntFundsSent(subcall))
	}
	return sent
}

func accruedShare(amount abi.TokenAmount, share uint64) abi.TokenAmount {
	return big.Div(big.Mul(amount, big.NewInt(int64(share))), big.NewInt(int64(reward19.Denom)))
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

// rewardEventsInRange fetches the reward actor's indexed events over one height range.
func rewardEventsInRange(ctx context.Context, t *testing.T, node api.FullNode, from, to abi.ChainEpoch) []*types.ActorEvent {
	t.Helper()
	events, err := node.GetActorEventsRaw(ctx, &types.ActorEventFilter{
		Addresses:  []address.Address{builtin.RewardActorAddr},
		FromHeight: &from,
		ToHeight:   &to,
	})
	require.NoError(t, err)
	return events
}

// requireActorEvent finds the one event whose leading entries match prefix, by key and encoded
// value.
func requireActorEvent(t *testing.T, events []*types.ActorEvent, prefix ...types.EventEntry) (int, *types.ActorEvent) {
	t.Helper()
	for i, event := range events {
		if len(event.Entries) < len(prefix) {
			continue
		}
		match := true
		for j, want := range prefix {
			if event.Entries[j].Key != want.Key || !bytes.Equal(event.Entries[j].Value, want.Value) {
				match = false
				break
			}
		}
		if match {
			return i, event
		}
	}
	require.FailNowf(t, "actor event not found", "prefix %+v", prefix)
	return -1, nil
}

// requireEventBigInt decodes a bigint event entry and checks it against want.
func requireEventBigInt(t *testing.T, entry types.EventEntry, key string, want abi.TokenAmount) {
	t.Helper()
	req := require.New(t)
	req.Equal(key, entry.Key)
	var got big.Int
	req.NoError(got.UnmarshalCBOR(bytes.NewReader(entry.Value)))
	req.Equalf(0, big.Cmp(want, got), "%s is %s, not %s", key, got, want)
}

// decodeShareRows decodes a shares-set event's shares entry into recipient ID to share.
func decodeShareRows(t *testing.T, entry types.EventEntry) map[uint64]uint64 {
	t.Helper()
	req := require.New(t)
	req.Equal("shares", entry.Key)
	node, err := ipld.Decode(entry.Value, dagcbor.Decode)
	req.NoError(err)
	rows := make(map[uint64]uint64, node.Length())
	it := node.ListIterator()
	for !it.Done() {
		_, row, err := it.Next()
		req.NoError(err)
		recipientNode, err := row.LookupByIndex(0)
		req.NoError(err)
		shareNode, err := row.LookupByIndex(1)
		req.NoError(err)
		recipient, err := recipientNode.AsInt()
		req.NoError(err)
		share, err := shareNode.AsInt()
		req.NoError(err)
		rows[uint64(recipient)] = uint64(share)
	}
	return rows
}

// shareRowsOf converts a wire share map to recipient-ID rows, matching decodeShareRows.
func shareRowsOf(t *testing.T, shares []reward19.RecipientShare) map[uint64]uint64 {
	t.Helper()
	req := require.New(t)
	rows := make(map[uint64]uint64, len(shares))
	for _, share := range shares {
		id, err := address.IDFromAddress(share.Recipient)
		req.NoError(err)
		rows[id] = share.Share
	}
	return rows
}

// requireSetSharesEvents checks a SetShares message's period-folded and shares-set events: the
// closed period's accrual and dust, and the newly installed map as actor-ID rows.
func requireSetSharesEvents(
	ctx context.Context, t *testing.T, node api.FullNode, height abi.ChainEpoch,
	id reward19.StreamID, accrued, dust abi.TokenAmount, shares []reward19.RecipientShare,
) {
	t.Helper()
	req := require.New(t)
	events := rewardEventsInRange(ctx, t, node, height, height)

	foldIdx, fold := requireActorEvent(t, events,
		eventEntry(t, 0x03, "$type", basicnode.NewString("period-folded")),
		eventEntry(t, 0x03, "stream-id", basicnode.NewInt(int64(id))),
		eventEntry(t, 0x01, "cause", basicnode.NewString("SetShares")),
	)
	req.Len(fold.Entries, 5)
	requireEventBigInt(t, fold.Entries[3], "accrued", accrued)
	requireEventBigInt(t, fold.Entries[4], "dust", dust)

	sharesIdx, sharesSet := requireActorEvent(t, events,
		eventEntry(t, 0x03, "$type", basicnode.NewString("shares-set")),
		eventEntry(t, 0x03, "stream-id", basicnode.NewInt(int64(id))),
	)
	req.Greater(sharesIdx, foldIdx, "shares-set must follow period-folded")
	req.Equal(shareRowsOf(t, shares), decodeShareRows(t, sharesSet.Entries[2]))
}

// requireAddressReplacedEvents checks a ReplaceAddress message's period-folded and
// address-replaced events, old and new recipients as actor IDs.
func requireAddressReplacedEvents(
	ctx context.Context, t *testing.T, node api.FullNode, height abi.ChainEpoch,
	id reward19.StreamID, old, new address.Address,
) {
	t.Helper()
	req := require.New(t)
	events := rewardEventsInRange(ctx, t, node, height, height)

	foldIdx, _ := requireActorEvent(t, events,
		eventEntry(t, 0x03, "$type", basicnode.NewString("period-folded")),
		eventEntry(t, 0x03, "stream-id", basicnode.NewInt(int64(id))),
		eventEntry(t, 0x01, "cause", basicnode.NewString("ReplaceAddress")),
	)

	oldID, err := address.IDFromAddress(old)
	req.NoError(err)
	newID, err := address.IDFromAddress(new)
	req.NoError(err)
	replacedIdx, _ := requireActorEvent(t, events,
		eventEntry(t, 0x03, "$type", basicnode.NewString("address-replaced")),
		eventEntry(t, 0x03, "stream-id", basicnode.NewInt(int64(id))),
		eventEntry(t, 0x03, "old-recipient", basicnode.NewInt(int64(oldID))),
		eventEntry(t, 0x03, "new-recipient", basicnode.NewInt(int64(newID))),
	)
	req.Greater(replacedIdx, foldIdx, "address-replaced must follow period-folded")
}

// requireRegistrationEventVisibility checks one query covering a registration from its queuing
// through the epoch it applies: the explicit message's write-queued is indexed, while the
// shares-set its application emits is not. AwardBlockReward is implicit and lotus's event index
// only walks messages in blocks, so the absence is the index, not a missing emission.
func requireRegistrationEventVisibility(ctx context.Context, t *testing.T, node api.FullNode, from, to abi.ChainEpoch, id reward19.StreamID) {
	t.Helper()
	events := rewardEventsInRange(ctx, t, node, from, to)
	requireActorEvent(t, events,
		eventEntry(t, 0x03, "$type", basicnode.NewString("write-queued")),
		eventEntry(t, 0x03, "op", basicnode.NewInt(int64(reward19.PendingWriteOpRegisterStream))),
	)
	sharesSetType := eventEntry(t, 0x03, "$type", basicnode.NewString("shares-set"))
	streamIDEntry := eventEntry(t, 0x03, "stream-id", basicnode.NewInt(int64(id)))
	for _, event := range events {
		if len(event.Entries) >= 2 &&
			bytes.Equal(event.Entries[0].Value, sharesSetType.Value) &&
			bytes.Equal(event.Entries[1].Value, streamIDEntry.Value) {
			require.FailNowf(t, "shares-set event must not be indexed from an implicit call",
				"found at height %d", event.Height)
		}
	}
}

// solsticeInvoke sends an EVM call and returns its receipt without requiring a successful exit
// code.
func solsticeInvoke(
	ctx context.Context,
	t *testing.T,
	client *kit.TestFullNode,
	from, to address.Address,
	selector []byte,
	args ...[]byte,
) *api.MsgLookup {
	t.Helper()
	var input []byte
	for _, arg := range args {
		input = append(input, arg...)
	}
	lookup, err := client.EVM().InvokeSolidity(ctx, from, to, selector, input)
	require.NoError(t, err)
	return lookup
}

// solsticePushMessage signs and pushes one message at an explicit nonce, so a nonce gap can hold it
// in the pool until the gap is filled. Gas is fixed because estimation refuses a failing message.
func solsticePushMessage(
	ctx context.Context,
	t *testing.T,
	client *kit.TestFullNode,
	from address.Address,
	nonce uint64,
	to address.Address,
	method abi.MethodNum,
	params cbg.CBORMarshaler,
) cid.Cid {
	t.Helper()
	req := require.New(t)
	var serialized []byte
	if params != nil {
		var err error
		serialized, err = actors.SerializeParams(params)
		req.NoError(err)
	}
	signed, err := client.WalletSignMessage(ctx, from, &types.Message{
		To: to, From: from, Nonce: nonce, Value: big.Zero(),
		Method: method, Params: serialized, GasLimit: 10_000_000,
		GasFeeCap: abi.NewTokenAmount(10_000), GasPremium: big.Zero(),
	})
	req.NoError(err)
	pushed, err := client.MpoolPush(ctx, signed)
	req.NoError(err)
	return pushed
}

// solsticePush sends an EVM call without waiting for inclusion.
func solsticePush(
	ctx context.Context,
	t *testing.T,
	client *kit.TestFullNode,
	from, to address.Address,
	selector []byte,
	args ...[]byte,
) cid.Cid {
	t.Helper()
	var params bytes.Buffer
	require.NoError(t, cbg.WriteByteArray(&params, solsticeCalldata(selector, args...)))
	signed, err := client.MpoolPushMessage(ctx, &types.Message{
		To: to, From: from, Value: big.Zero(),
		Method: builtin.MethodsEVM.InvokeContract,
		// A quarter of the block leaves room for both approvals in one block.
		GasLimit: buildconstants.BlockGasLimit / 4,
		Params:   params.Bytes(),
	}, nil)
	require.NoError(t, err)
	return signed.Cid()
}

// solsticeLaterLookup returns the lookup whose tipset's parent state includes both approvals.
func solsticeLaterLookup(first, second *api.MsgLookup) *api.MsgLookup {
	if second.Height >= first.Height {
		return second
	}
	return first
}

// solsticeWait resolves a pushed call's receipt with the exit code left to the caller.
func solsticeWait(ctx context.Context, t *testing.T, client *kit.TestFullNode, message cid.Cid) *api.MsgLookup {
	t.Helper()
	lookup, err := client.StateWaitMsg(ctx, message, 1, api.LookbackNoLimit, false)
	require.NoError(t, err)
	return lookup
}

func solsticeCalldata(selector []byte, args ...[]byte) []byte {
	calldata := append([]byte(nil), selector...)
	for _, arg := range args {
		calldata = append(calldata, arg...)
	}
	return calldata
}

// solsticeTaskID is the identifier a unanimous call is recorded under, keccak256 of its calldata.
func solsticeTaskID(selector []byte, args ...[]byte) []byte {
	hasher := keccak.NewLegacyKeccak256()
	hasher.Write(solsticeCalldata(selector, args...))
	return hasher.Sum(nil)
}

func solsticeSetWeightRecords(t *testing.T) []byte {
	t.Helper()
	return solsticeSelector(t, "StreamWeightActor", "setWeightRecords((uint64,(int256,int256,uint64,int256,int256))[])")
}

// solsticeWord decodes a successful call's single ABI-encoded 32-byte return word.
func solsticeWord(t *testing.T, lookup *api.MsgLookup) []byte {
	t.Helper()
	kit.RequireMessageSuccess(t, lookup)
	out, err := cbg.ReadByteArray(bytes.NewBuffer(lookup.Receipt.Return), uint64(len(lookup.Receipt.Return)))
	require.NoError(t, err)
	require.Len(t, out, 32)
	return out
}

// solsticeBool decodes a successful call's single ABI-encoded bool.
func solsticeBool(t *testing.T, lookup *api.MsgLookup) bool {
	t.Helper()
	return solsticeWord(t, lookup)[31] != 0
}

func solsticeIDAddress(t *testing.T, id uint64) address.Address {
	t.Helper()
	addr, err := address.NewIDAddress(id)
	require.NoError(t, err)
	return addr
}

// solsticeWeightWords encodes a WeightRecord as vStart, slope, tStart, floor, cap.
func solsticeWeightWords(record reward19.WeightRecord) []byte {
	words := kit.EvmWordUint64(record.VStart)
	words = append(words, kit.EvmWordInt64(record.Slope)...)
	words = append(words, kit.EvmWordUint64(uint64(record.TStart))...)
	words = append(words, kit.EvmWordUint64(record.Floor)...)
	return append(words, kit.EvmWordUint64(record.Cap)...)
}

// solsticeWeightUpdatesInput encodes an offset word, the array length and six-word (id, record)
// tuples.
func solsticeWeightUpdatesInput(updates []reward19.WeightRecordUpdate) []byte {
	input := kit.EvmWordUint64(32)
	input = append(input, kit.EvmWordUint64(uint64(len(updates)))...)
	for _, update := range updates {
		input = append(input, kit.EvmWordUint64(uint64(update.ID))...)
		input = append(input, solsticeWeightWords(update.Weight)...)
	}
	return input
}

// registerStream's nine-word head is id, WeightRecord, writer, shares offset and activation; the
// tail is the share count then (wallet, share) pairs.
func solsticeRegisterStreamInput(
	ctx context.Context,
	t *testing.T,
	client *kit.TestFullNode,
	id reward19.StreamID,
	weight reward19.WeightRecord,
	writer address.Address,
	shares []reward19.RecipientShare,
	activation abi.ChainEpoch,
) []byte {
	t.Helper()
	const headWords = 9
	input := kit.EvmWordUint64(uint64(id))
	input = append(input, solsticeWeightWords(weight)...)
	input = append(input, kit.EvmWordFromAddr(ctx, t, client, writer)...)
	input = append(input, kit.EvmWordUint64(headWords*32)...)
	input = append(input, kit.EvmWordUint64(uint64(activation))...)
	input = append(input, kit.EvmWordUint64(uint64(len(shares)))...)
	for _, share := range shares {
		input = append(input, kit.EvmWordFromAddr(ctx, t, client, share.Recipient)...)
		input = append(input, kit.EvmWordUint64(share.Share)...)
	}
	return input
}

const solsticeBundleDir = "contracts/solstice"

var solsticeUintType = regexp.MustCompile(`^uint(\d+)$`)

// solsticeArg pairs a constructor argument with its Solidity type for the manifest check.
type solsticeArg struct {
	typ string
	// data holds a static 32-byte word or an unpadded dynamic payload.
	data    []byte
	dynamic bool
}

func solsticeAddress(word []byte) solsticeArg {
	return solsticeArg{typ: "address", data: word}
}

func solsticeUint64(n uint64) solsticeArg {
	return solsticeArg{typ: "uint64", data: kit.EvmWordUint64(n)}
}

// solsticeBytes marks payload as a dynamic bytes constructor argument.
func solsticeBytes(payload []byte) solsticeArg {
	return solsticeArg{typ: "bytes", data: payload, dynamic: true}
}

type solsticeContract struct {
	CreationBytecode          string            `json:"creationBytecode"`
	ConstructorParameterTypes []string          `json:"constructorParameterTypes"`
	FunctionSelectors         map[string]string `json:"functionSelectors"`
}

func solsticeManifestEntry(t *testing.T, contract string) solsticeContract {
	t.Helper()

	var bundle struct {
		Contracts map[string]solsticeContract `json:"contracts"`
	}
	raw, err := os.ReadFile(filepath.Join(solsticeBundleDir, "manifest.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &bundle))

	entry, ok := bundle.Contracts[contract]
	require.Truef(t, ok, "%s missing from manifest.json", contract)
	return entry
}

// solsticeSelector computes sig's selector and checks the contract manifest declares it.
func solsticeSelector(t *testing.T, contract, sig string) []byte {
	t.Helper()

	declared, ok := solsticeManifestEntry(t, contract).FunctionSelectors[sig]
	require.Truef(t, ok, "%s does not declare %q in manifest.json", contract, sig)
	selector := kit.EthFunctionHash(sig)
	require.Equalf(t, declared, "0x"+hex.EncodeToString(selector),
		"%s selector for %q disagrees with manifest.json", contract, sig)
	return selector
}

// solsticeCreationCode appends the ABI-encoded args to the creation bytecode after checking
// their types.
func solsticeCreationCode(t *testing.T, contract string, args ...solsticeArg) []byte {
	t.Helper()

	entry := solsticeManifestEntry(t, contract)
	declared := entry.ConstructorParameterTypes
	encoded := make([]string, len(args))
	for i, arg := range args {
		encoded[i] = arg.typ
	}
	require.Equalf(t, declared, encoded,
		"%s constructor arguments do not match manifest.json's parameter types", contract)

	encodedFile, err := os.ReadFile(filepath.Join(solsticeBundleDir, entry.CreationBytecode))
	require.NoError(t, err)
	creationCode, err := hex.DecodeString(string(bytes.TrimSpace(encodedFile)))
	require.NoError(t, err)

	return append(creationCode, solsticeEncodeArgs(t, contract, args)...)
}

// solsticeEncodeArgs writes static words and dynamic offsets, then length-prefixed, 32-byte-padded
// tails.
func solsticeEncodeArgs(t *testing.T, contract string, args []solsticeArg) []byte {
	t.Helper()

	headSize := 32 * len(args)
	head := make([]byte, 0, headSize)
	var tail []byte
	for i, arg := range args {
		if arg.dynamic {
			head = append(head, kit.EvmWordUint64(uint64(headSize+len(tail)))...)
			tail = append(tail, kit.EvmWordUint64(uint64(len(arg.data)))...)
			tail = append(tail, arg.data...)
			if remainder := len(arg.data) % 32; remainder != 0 {
				tail = append(tail, make([]byte, 32-remainder)...)
			}
			continue
		}

		require.Lenf(t, arg.data, 32, "%s constructor argument %d is not a 32-byte word", contract, i)
		var usedBytes int
		switch {
		case arg.typ == "address":
			usedBytes = 20
		case solsticeUintType.MatchString(arg.typ):
			bits, err := strconv.Atoi(solsticeUintType.FindStringSubmatch(arg.typ)[1])
			require.NoError(t, err)
			usedBytes = bits / 8
		default:
			t.Fatalf("%s constructor argument %d has unhandled type %q", contract, i, arg.typ)
		}
		for _, b := range arg.data[:32-usedBytes] {
			require.Zerof(t, b, "%s constructor argument %d (%s) uses more than its declared %d bytes",
				contract, i, arg.typ, usedBytes)
		}
		head = append(head, arg.data...)
	}
	return append(head, tail...)
}
