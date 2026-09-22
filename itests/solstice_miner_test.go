package itests

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
	lcli "github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	verifreg13 "github.com/filecoin-project/go-state-types/builtin/v13/verifreg"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	verifreg14 "github.com/filecoin-project/go-state-types/builtin/v14/verifreg"
	datacap19 "github.com/filecoin-project/go-state-types/builtin/v19/datacap"
	stminer "github.com/filecoin-project/go-state-types/builtin/v19/miner"
	verifreg19 "github.com/filecoin-project/go-state-types/builtin/v19/verifreg"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/manifest"
	"github.com/filecoin-project/go-state-types/network"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/datacap"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	lminer "github.com/filecoin-project/lotus/cli/miner"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/must"
)

// Sector size for every miner here, and the unit power assertions count in.
const solsticeSectorSize = abi.SectorSize(2 << 10) // 2KiB

// An allocation id the verified registry never issued.
const solsticeUnknownAllocation = verifreg14.AllocationId(1 << 40)

// solsticeLifecycle is the shared fixture: one chain, three unmanaged miners.
//
//	mixed  9 legacy CC, a precommit held across the fork, and native CC sectors
//	deals  2 verified and 2 unverified legacy deals, 2 legacy CC (one snapped to verified), 3 native deals
//	cli    6 legacy CC, upgraded as a set by lotus-miner sectors upgrade-quality
type solsticeLifecycle struct {
	ctx          context.Context
	client       *kit.TestFullNode
	mixed        *kit.TestUnmanagedMiner // legacy CC sectors, the subject of the quality upgrades
	deals        *kit.TestUnmanagedMiner // verified and unverified deal sectors
	cli          *kit.TestUnmanagedMiner // legacy CC sectors the upgrade-quality CLI upgrades as a set
	sealProof    abi.RegisteredSealProof
	upgradeEpoch abi.ChainEpoch
	migrated     types.TipSetKey // the first tipset that reads migrated state

	verifiedClient address.Address // the account that holds allocV1, allocV2 and allocSnap
	pieceV         abi.PieceInfo   // the piece allocV1 and allocV2 were made against

	mL       []abi.SectorNumber // legacy CC
	mP1      abi.SectorNumber   // precommitted on NV28, proven on NV29
	dV1, dV2 abi.SectorNumber   // legacy verified deals
	dU1, dU2 abi.SectorNumber   // legacy unverified deals
	dC1, dC2 abi.SectorNumber   // legacy CC, dC2 snapped into a verified deal before the fork
	mN3      abi.SectorNumber   // native CC
	dD1, dD2 abi.SectorNumber   // native deals pointing to a dangling allocation, unknown and already claimed
	cL       []abi.SectorNumber // legacy CC

	// The verified allocations the deal sectors claim, one per sector.
	allocV1Client, allocV2Client, allocSnapClient abi.ActorID
	allocV1, allocV2, allocSnap                   verifreg13.AllocationId
}

// TestSolsticeMinerLifecycle drives FIP-0118's miner rules through one chain: three unmanaged miners
// onboard on NV28, cross into NV29, and are then read and upgraded in place.
func TestSolsticeMinerLifecycle(t *testing.T) {
	req := require.New(t)

	// App() sets the process-wide node type and resets the log levels, so it runs before the test
	// quiets them.
	wasNodeType := lapi.RunningNodeType
	minerApp := lminer.App()
	lapi.RunningNodeType = lapi.NodeMiner
	t.Cleanup(func() { lapi.RunningNodeType = wasNodeType })

	kit.QuietMiningLogs()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Every sector onboarded before the fork must have its first WindowPoSt in before it: claimed power
	// starts at that post, and the migration reads compare power across the fork. First post by
	// ChainFinality + one proving period; the rest covers the pre-fork snap and extensions, which wait
	// for a mutable deadline each and are serial on the deals miner.
	upgradeEpoch := miner14.ChainFinality + miner.WPoStProvingPeriod() + 12*miner.WPoStChallengeWindow()
	t.Logf("NV28 to NV29 at epoch %d", upgradeEpoch)

	sealProof, err := miner.SealProofTypeFromSectorSize(solsticeSectorSize, network.Version28, miner.SealProofVariant_Standard)
	req.NoError(err)

	// SetupVerifiedClients needs a root verifier and funded verifier and client accounts.
	bal := types.FromFil(1000)
	rootKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifierKey := must.One(key.GenerateKey(types.KTSecp256k1))
	clientKey := must.One(key.GenerateKey(types.KTBLS))
	mixedKey := must.One(key.GenerateKey(types.KTBLS))
	dealsKey := must.One(key.GenerateKey(types.KTBLS))
	cliKey := must.One(key.GenerateKey(types.KTBLS))

	var client kit.TestFullNode
	var producer kit.TestMiner
	ens := kit.NewEnsemble(t,
		kit.MockProofs(),
		kit.RootVerifier(rootKey, bal),
		kit.Account(verifierKey, bal),
		kit.Account(clientKey, bal),
		kit.Account(mixedKey, bal),
		kit.Account(dealsKey, bal),
		kit.Account(cliKey, bal),
		kit.UpgradeSchedule(
			stmgr.Upgrade{Network: network.Version28, Height: -1}, // genesis is NV28
			stmgr.Upgrade{
				Network:   network.Version29,
				Height:    upgradeEpoch,
				Migration: filcns.UpgradeActorsV19With(buildconstants.NeutralSolsticeRewardBootstrapParams),
			},
		),
	).
		FullNode(&client, kit.SectorSize(solsticeSectorSize), kit.ThroughRPC()).
		// The block producer holds ~15% of network quality-adjusted power once every unmanaged sector
		// is at 10x, so blocks keep arriving inside each challenge window.
		Miner(&producer, &client, kit.PresealSectors(4), kit.SectorSize(solsticeSectorSize), kit.WithAllSubsystems()).
		Start().
		InterconnectAll()

	blockMiners := ens.BeginMiningMustPost(5 * time.Millisecond)
	req.Len(blockMiners, 1)
	blockMiner := blockMiners[0]

	minerOpts := func(k *key.Key) []kit.NodeOpt {
		return []kit.NodeOpt{kit.SectorSize(solsticeSectorSize), kit.OwnerAddr(k)}
	}
	mixed, ens := ens.UnmanagedMiner(ctx, &client, minerOpts(mixedKey)...)
	defer mixed.Stop()
	deals, ens := ens.UnmanagedMiner(ctx, &client, minerOpts(dealsKey)...)
	defer deals.Stop()
	cliMiner, ens := ens.UnmanagedMiner(ctx, &client, minerOpts(cliKey)...)
	defer cliMiner.Stop()
	ctx = ens.UnmanagedContext(ctx)
	ens.Start()

	// Watch every miner from the moment it exists, so no miner's first window goes unprotected. A
	// miner with no sectors has nothing to prove, so watching it early costs nothing.
	for _, m := range []*kit.TestUnmanagedMiner{mixed, deals, cliMiner} {
		blockMiner.WatchMinerForPost(m.ActorAddr)
	}

	// --actor is an app-level flag of lotus-miner, so the mock app needs it alongside --api-url.
	minerCLI := kit.NewMockCLI(ctx, t, minerApp.Commands, lapi.NodeMiner,
		&lcli.StringFlag{Name: "actor"}).Client(client.ListenAddr)

	f := &solsticeLifecycle{
		ctx: ctx, client: &client,
		mixed: mixed, deals: deals, cli: cliMiner,
		sealProof: sealProof, upgradeEpoch: upgradeEpoch,
	}

	var pieceV, pieceSnap abi.PieceInfo
	run := func(name string, f func(*testing.T)) bool {
		return t.Run(name, func(t *testing.T) {
			if err := context.Cause(ctx); err != nil {
				t.Fatal(err)
			}
			f(t)
			if err := context.Cause(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}

	if !run("the upgrade-quality CLI refuses NV28", func(t *testing.T) {
		_, err := minerCLI.RunCmdRaw("sectors", "upgrade-quality", "--actor="+f.cli.ActorAddr.String())
		require.ErrorContains(t, err, "requires network version 29+")
	}) {
		return
	}
	if !run("a verified client holds allocations before the fork", func(t *testing.T) {
		pieceV, pieceSnap = f.setupVerifiedFixture(t, rootKey, verifierKey, clientKey)
	}) {
		return
	}
	if !run("legacy sectors of every content type gain power on NV28", func(t *testing.T) {
		f.onboardBeforeFork(t, pieceV, pieceSnap)
	}) {
		return
	}
	if !run("the migration leaves every legacy sector as it was", f.crossTheFork) {
		return
	}
	if !run("datacap and verifreg writes are refused", func(t *testing.T) {
		requireSolsticeFrozenActors(ctx, t, &client, f.migrated)
	}) {
		return
	}
	if !run("sectors activated on NV29 are created at full quality", f.nativeSectors) {
		return
	}
	if !run("a sector using a dangling allocation is created at full quality", f.danglingAllocations) {
		return
	}
	if !run("the upgrade-quality CLI packs one miner's sectors into capped messages", func(t *testing.T) {
		f.upgradeQualityCLI(t, minerCLI)
	}) {
		return
	}
	if !run("upgrading a legacy sector raises it once and only once", f.upgradeSectorQuality) {
		return
	}
	if !run("an extension moves the expiration and re-derives the weights", f.extendSectors) {
		return
	}
	if !run("snapping and upgrading reach 10x in either order", f.snapOrdering) {
		return
	}
	if !run("a deal sector rises only if its content did not already pay for it", f.dealTiers) {
		return
	}
	if !run("the proving window refuses a termination and allows an upgrade", f.deadlineImmutability) {
		return
	}
	if !run("only the owner, worker and control addresses may upgrade a sector", f.upgradeAuthorization) {
		return
	}
	if !run("terminating a sector removes exactly its own power", f.terminations) {
		return
	}
}

// setupVerifiedFixture creates the verifier, the verified client and the three allocations the deal
// sectors claim. FIP-0118 refuses every one of these writes from NV29, so they happen before the fork.
func (f *solsticeLifecycle) setupVerifiedFixture(t *testing.T, rootKey, verifierKey, clientKey *key.Key) (pieceV, pieceSnap abi.PieceInfo) {
	req := require.New(t)

	_, clients := kit.SetupVerifiedClients(f.ctx, t, f.client, rootKey, verifierKey, []*key.Key{clientKey})
	req.Len(clients, 1)
	clientAddr := clients[0]
	f.verifiedClient = clientAddr

	dealsID := must.One(address.IDFromAddress(f.deals.ActorAddr))
	pieceV = abi.PieceInfo{Size: abi.PaddedPieceSize(solsticeSectorSize), PieceCID: kit.BogusPieceCid2}
	pieceSnap = abi.PieceInfo{Size: abi.PaddedPieceSize(solsticeSectorSize), PieceCID: kit.BogusPieceCid1}
	f.pieceV = pieceV

	f.allocV1Client, f.allocV1 = kit.SetupAllocation(f.ctx, t, f.client, dealsID, pieceV, clientAddr, 0, 0)
	f.allocV2Client, f.allocV2 = kit.SetupAllocation(f.ctx, t, f.client, dealsID, pieceV, clientAddr, 0, 0)
	f.allocSnapClient, f.allocSnap = kit.SetupAllocation(f.ctx, t, f.client, dealsID, pieceSnap, clientAddr, 0, 0)
	req.NotEqual(f.allocV1, f.allocV2, "each verified sector claims its own allocation")

	return pieceV, pieceSnap
}

// onboardBeforeFork onboards every miner's sectors concurrently, then makes the changes that have to
// happen on NV28: the snap into a verified deal, the extensions read back across the fork, and the
// precommit whose prove lands on the other side.
func (f *solsticeLifecycle) onboardBeforeFork(t *testing.T, pieceV, pieceSnap abi.PieceInfo) {
	req := require.New(t)

	verifiedSector := func(piece abi.PieceInfo, client abi.ActorID, alloc verifreg14.AllocationId) kit.SectorManifest {
		return kit.SectorWithVerifiedPiece(piece.PieceCID, &miner14.VerifiedAllocationKey{Client: client, ID: alloc})
	}

	// One goroutine per miner: the first OnboardSectors of each blocks a whole challenge window
	// before its post loop starts, and those three windows are the same window.
	var mSectors, dSectors, cSectors []abi.SectorNumber
	var eg errgroup.Group
	eg.Go(func() error {
		mSectors, _ = f.mixed.OnboardSectors(f.sealProof, kit.NewSectorBatch().AddEmptySectors(9))
		return nil
	})
	eg.Go(func() error {
		dSectors, _ = f.deals.OnboardSectors(f.sealProof, kit.NewSectorBatch().
			AddSector(verifiedSector(pieceV, f.allocV1Client, verifreg14.AllocationId(f.allocV1))).
			AddSector(verifiedSector(pieceV, f.allocV2Client, verifreg14.AllocationId(f.allocV2))).
			AddSectorsWithRandomPieces(2).
			AddEmptySectors(2))
		return nil
	})
	eg.Go(func() error {
		cSectors, _ = f.cli.OnboardSectors(f.sealProof, kit.NewSectorBatch().AddEmptySectors(6))
		return nil
	})
	req.NoError(eg.Wait())
	req.NoError(context.Cause(f.ctx))
	// A require failure inside the kit unwinds only its own goroutine, so Wait returning nil proves
	// nothing on its own: check that each miner actually came back with its sectors.
	req.Len(mSectors, 9, "the mixed miner did not finish onboarding")
	req.Len(dSectors, 6, "the deals miner did not finish onboarding")
	req.Len(cSectors, 6, "the cli miner did not finish onboarding")

	f.mL = mSectors
	f.dV1, f.dV2 = dSectors[0], dSectors[1]
	f.dU1, f.dU2 = dSectors[2], dSectors[3]
	f.dC1, f.dC2 = dSectors[4], dSectors[5]
	f.cL = cSectors

	// Every pre-fork sector has its first post in, so its power is claimed and the migration reads
	// have something to compare.
	unit := uint64(solsticeSectorSize)
	req.NoError(f.mixed.WaitTillActivatedAndAssertPower(mSectors, unit*9, unit*9))
	req.NoError(f.deals.WaitTillActivatedAndAssertPower(dSectors, unit*6, unit*(10+10+1+1+1+1)))
	req.NoError(f.cli.WaitTillActivatedAndAssertPower(cSectors, unit*6, unit*6))

	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	t.Logf("every pre-fork sector is proven at epoch %d, %d epochs before the fork",
		head.Height(), f.upgradeEpoch-head.Height())

	// Legacy tiers, read once for every sector.
	for _, sn := range mSectors {
		info := f.sectorInfo(t, f.mixed.ActorAddr, sn, head.Key())
		req.Less(info.Activation, f.upgradeEpoch, "legacy CC sector %d must activate before the fork", sn)
		req.Zero(info.Flags&miner.FULL_QA_POWER, "legacy CC sector %d must not carry FULL_QA_POWER", sn)
	}
	for _, sn := range []abi.SectorNumber{f.dV1, f.dV2} {
		info := f.sectorInfo(t, f.deals.ActorAddr, sn, head.Key())
		req.Zero(info.Flags&miner.FULL_QA_POWER, "verified sector %d is 10x by weight, not by flag", sn)
		req.Positive(info.VerifiedDealWeight.Int64(), "verified sector %d must carry verified weight", sn)
		req.True(miner.SectorIsFullQaPower(info), "verified sector %d must classify as full-QA", sn)
		req.GreaterOrEqual(info.PowerBaseEpoch, info.Activation, "PowerBaseEpoch must not precede Activation")
	}
	for _, sn := range []abi.SectorNumber{f.dU1, f.dU2} {
		info := f.sectorInfo(t, f.deals.ActorAddr, sn, head.Key())
		req.Zero(info.Flags&miner.FULL_QA_POWER, "unverified deal sector %d must not carry FULL_QA_POWER", sn)
		req.Zero(info.VerifiedDealWeight.Int64(), "unverified deal sector %d must carry no verified weight", sn)
		req.Positive(info.DealWeight.Int64(), "unverified deal sector %d must carry its 1x deal weight", sn)
	}
	for _, sn := range []abi.SectorNumber{f.dC1, f.dC2} {
		info := f.sectorInfo(t, f.deals.ActorAddr, sn, head.Key())
		req.False(miner.SectorIsFullQaPower(info), "empty sector %d must classify as 1x", sn)
	}
	f.requireHelperMatchesPower(t, f.deals.ActorAddr, head.Key())

	// A snap resets a PowerBaseEpoch and an extension moves an expiration, both on NV28, so the
	// migration reads cover one of each.
	f.deals.SnapDeal(f.dC2, kit.SectorWithVerifiedPiece(pieceSnap.PieceCID,
		&miner14.VerifiedAllocationKey{Client: f.allocSnapClient, ID: verifreg14.AllocationId(f.allocSnap)}))

	kit.WaitForMinerQAP(f.ctx, t, f.client, f.deals.ActorAddr, unit*(10+10+1+1+1+10), 3*time.Minute)
	head, err = f.client.ChainHead(f.ctx)
	req.NoError(err)
	snapped := f.sectorInfo(t, f.deals.ActorAddr, f.dC2, head.Key())
	req.True(miner.SectorIsFullQaPower(snapped),
		"miner %s sector %d, snapped into a verified deal, must classify as full-QA", f.deals.ActorAddr, f.dC2)
	req.Zero(snapped.Flags&miner.FULL_QA_POWER,
		"miner %s sector %d is 10x by weight, not by flag", f.deals.ActorAddr, f.dC2)
	f.requireHelperMatchesPower(t, f.deals.ActorAddr, head.Key())

	v2Before := f.sectorInfo(t, f.deals.ActorAddr, f.dV2, head.Key())
	v2Target := v2Before.Expiration + builtin.EpochsInDay
	f.deals.ExtendSectorExpiration(f.dV2, v2Target, verifreg14.ClaimId(f.allocV2))
	l4Before := f.sectorInfo(t, f.mixed.ActorAddr, f.mL[3], head.Key())
	l4Target := l4Before.Expiration + builtin.EpochsInDay
	f.mixed.ExtendSectorExpiration(f.mL[3], l4Target)

	head, err = f.client.ChainHead(f.ctx)
	req.NoError(err)
	v2After := f.sectorInfo(t, f.deals.ActorAddr, f.dV2, head.Key())
	req.Equal(v2Target, v2After.Expiration,
		"miner %s sector %d must expire where the extension asked", f.deals.ActorAddr, f.dV2)
	req.Positive(v2After.VerifiedDealWeight.Int64(),
		"miner %s sector %d must keep verified weight through an extension", f.deals.ActorAddr, f.dV2)
	req.True(miner.SectorIsFullQaPower(v2After),
		"miner %s sector %d must still classify as full-QA after its extension", f.deals.ActorAddr, f.dV2)
	f.requireHelperMatchesPower(t, f.deals.ActorAddr, head.Key())

	l4After := f.sectorInfo(t, f.mixed.ActorAddr, f.mL[3], head.Key())
	req.Equal(l4Target, l4After.Expiration,
		"miner %s sector %d must expire where the extension asked", f.mixed.ActorAddr, f.mL[3])
	req.Zero(l4After.Flags&miner.FULL_QA_POWER,
		"an extension before the fork must not promote miner %s sector %d", f.mixed.ActorAddr, f.mL[3])
	req.Zero(l4After.DealWeight.Int64(),
		"miner %s sector %d holds no deal, so an extension cannot give it deal weight", f.mixed.ActorAddr, f.mL[3])
	req.Zero(l4After.VerifiedDealWeight.Int64(),
		"miner %s sector %d holds no claim, so an extension cannot give it verified weight", f.mixed.ActorAddr, f.mL[3])

	// This precommit is proven after the fork, where activation decides its tier.
	held, err := f.mixed.PreCommitSectors(f.sealProof, kit.NewSectorBatch().AddEmptySectors(1))
	req.NoError(err)
	req.Len(held, 1)
	f.mP1 = held[0]
	notYet, err := f.client.StateSectorGetInfo(f.ctx, f.mixed.ActorAddr, f.mP1, types.EmptyTSK)
	req.NoError(err)
	req.Nil(notYet, "a precommitted sector is not committed until it is proven")

	head, err = f.client.ChainHead(f.ctx)
	req.NoError(err)
	req.Less(head.Height(), f.upgradeEpoch,
		"onboarding must finish before the fork at %d; it ran to %d", f.upgradeEpoch, head.Height())
	nv, err := f.client.StateNetworkVersion(f.ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version28, nv, "every pre-fork assertion must be made on NV28")
}

// crossTheFork waits for NV29, records the first migrated tipset as f.migrated and reads it.
func (f *solsticeLifecycle) crossTheFork(t *testing.T) {
	req := require.New(t)

	type sectorState struct {
		miner address.Address
		info  *miner.SectorOnChainInfo
	}
	before := make(map[abi.SectorNumber]sectorState)
	powerBefore := make(map[address.Address]lapi.MinerPower)

	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	for _, m := range []*kit.TestUnmanagedMiner{f.mixed, f.deals, f.cli} {
		pw, err := f.client.StateMinerPower(f.ctx, m.ActorAddr, head.Key())
		req.NoError(err)
		powerBefore[m.ActorAddr] = *pw
		for _, sn := range f.sectorsOf(t, m) {
			before[sn] = sectorState{miner: m.ActorAddr, info: f.sectorInfo(t, m.ActorAddr, sn, head.Key())}
		}
	}

	verifiedStatusBefore, err := f.client.StateVerifiedClientStatus(f.ctx, f.verifiedClient, head.Key())
	req.NoError(err)
	allocationsBefore, err := f.client.StateGetAllocations(f.ctx, f.verifiedClient, head.Key())
	req.NoError(err)
	datacapBefore := f.datacapBalance(t, f.verifiedClient, head.Key())

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(f.upgradeEpoch+5))
	head, err = f.client.ChainHead(f.ctx)
	req.NoError(err)
	nv, err := f.client.StateNetworkVersion(f.ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "the chain must be on NV29 after the migration")

	// A height past the fork does not by itself mean this tipset's parent state has been migrated, so
	// the reads below, and the frozen entry points that reuse this tipset, wait for a migrated actor.
	v19Reward, ok := actors.GetActorCodeID(actorstypes.Version19, manifest.RewardKey)
	req.True(ok, "the v19 manifest must name a reward actor")
	for head.Height() < f.upgradeEpoch+miner.WPoStChallengeWindow() {
		reward, err := f.client.StateGetActor(f.ctx, builtin.RewardActorAddr, head.Key())
		req.NoError(err)
		if reward.Code == v19Reward {
			break
		}
		f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(head.Height()+1))
		head, err = f.client.ChainHead(f.ctx)
		req.NoError(err)
	}
	reward, err := f.client.StateGetActor(f.ctx, builtin.RewardActorAddr, head.Key())
	req.NoError(err)
	req.Equal(v19Reward, reward.Code, "this tipset must read migrated state, not a pre-upgrade parent")

	// The migration touches neither verifreg nor datacap, so their state reads as before the fork.
	verifiedStatusAfter, err := f.client.StateVerifiedClientStatus(f.ctx, f.verifiedClient, head.Key())
	req.NoError(err)
	req.Equal(verifiedStatusBefore.String(), verifiedStatusAfter.String(),
		"client %s's verified status must be unchanged by the migration", f.verifiedClient)
	allocationsAfter, err := f.client.StateGetAllocations(f.ctx, f.verifiedClient, head.Key())
	req.NoError(err)
	req.Equal(allocationsBefore, allocationsAfter,
		"client %s's allocations must be unchanged by the migration", f.verifiedClient)
	datacapAfter := f.datacapBalance(t, f.verifiedClient, head.Key())
	req.Equal(datacapBefore.String(), datacapAfter.String(),
		"client %s's datacap balance must be unchanged by the migration", f.verifiedClient)
	claim, err := f.client.StateGetClaim(f.ctx, f.deals.ActorAddr, verifreg.ClaimId(f.allocV1), head.Key())
	req.NoError(err)
	req.NotNil(claim, "miner %s's claim %d, for sector %d, must still serve after the migration",
		f.deals.ActorAddr, f.allocV1, f.dV1)

	for sn, was := range before {
		now := f.sectorInfo(t, was.miner, sn, head.Key())
		req.Equal(was.info.DealWeight, now.DealWeight, "sector %d deal weight changed at the migration", sn)
		req.Equal(was.info.VerifiedDealWeight, now.VerifiedDealWeight, "sector %d verified weight changed", sn)
		req.Equal(was.info.Flags, now.Flags, "sector %d flags changed at the migration", sn)
		req.Equal(was.info.Activation, now.Activation, "sector %d activation changed", sn)
		req.Equal(was.info.Expiration, now.Expiration, "sector %d expiration changed", sn)
		req.Zero(now.Flags&miner.FULL_QA_POWER, "the migration must not set FULL_QA_POWER on sector %d", sn)
		req.Equal(miner.SectorIsFullQaPower(was.info), miner.SectorIsFullQaPower(now),
			"sector %d changed full-QA classification at the migration", sn)
	}

	for _, m := range []*kit.TestUnmanagedMiner{f.mixed, f.deals, f.cli} {
		pw, err := f.client.StateMinerPower(f.ctx, m.ActorAddr, head.Key())
		req.NoError(err)
		was := powerBefore[m.ActorAddr]
		req.Equal(was.MinerPower.RawBytePower, pw.MinerPower.RawBytePower, "miner %s raw power changed", m.ActorAddr)
		req.Equal(was.MinerPower.QualityAdjPower, pw.MinerPower.QualityAdjPower,
			"miner %s QA power changed at the migration: FIP-0118 is not retroactive", m.ActorAddr)
	}

	// The read paths a CLI uses keep working on migrated state.
	sectors, err := f.client.StateMinerSectors(f.ctx, f.mixed.ActorAddr, nil, head.Key())
	req.NoError(err)
	req.Len(sectors, len(f.mL), "miner %s must still list its sectors", f.mixed.ActorAddr)
	listed := make(map[abi.SectorNumber]*miner.SectorOnChainInfo, len(sectors))
	for _, info := range sectors {
		listed[info.SectorNumber] = info
	}
	for _, sn := range f.mL {
		info, ok := listed[sn]
		req.True(ok, "miner %s sector %d must appear in the listing", f.mixed.ActorAddr, sn)
		req.Zero(info.Flags&miner.FULL_QA_POWER,
			"miner %s sector %d must still read as a legacy sector", f.mixed.ActorAddr, sn)
		req.Equal(before[sn].info.VerifiedDealWeight, info.VerifiedDealWeight,
			"miner %s sector %d must list the verified weight it had", f.mixed.ActorAddr, sn)
	}
	dl, err := f.client.StateMinerProvingDeadline(f.ctx, f.mixed.ActorAddr, head.Key())
	req.NoError(err)
	req.NotNil(dl)
	deadlines, err := f.client.StateMinerDeadlines(f.ctx, f.mixed.ActorAddr, head.Key())
	req.NoError(err)
	req.NotNil(deadlines)

	stillHeld, err := f.client.StateSectorGetInfo(f.ctx, f.mixed.ActorAddr, f.mP1, head.Key())
	req.NoError(err)
	req.Nil(stillHeld, "the held precommit must still be unproven after the migration")

	f.mixed.AssertNoWindowPostError()
	f.deals.AssertNoWindowPostError()
	f.cli.AssertNoWindowPostError()

	f.migrated = head.Key()
}

// nativeSectors proves the held precommit, runs a native precommit through its deposit accounting, and
// onboards the native sectors. Everything is submitted before the single power wait, because each new
// sector's power arrives at its own first post, up to a proving period away.
func (f *solsticeLifecycle) nativeSectors(t *testing.T) {
	req := require.New(t)
	unit := uint64(solsticeSectorSize)

	// The precommit taken on NV28 activates on NV29, and activation decides the tier.
	proven, err := f.mixed.ProvePrecommittedSectors(f.sealProof, []abi.SectorNumber{f.mP1})
	req.NoError(err)
	req.Equal([]abi.SectorNumber{f.mP1}, proven, "the held precommit must prove")

	p1 := f.sectorInfo(t, f.mixed.ActorAddr, f.mP1, types.EmptyTSK)
	req.GreaterOrEqual(p1.Activation, f.upgradeEpoch, "the held sector activates on NV29")
	req.NotZero(p1.Flags&miner.FULL_QA_POWER,
		"a sector precommitted on NV28 and proven on NV29 must carry FULL_QA_POWER: activation decides the tier")

	// A precommit taken and proven on NV29 reserves a deposit and releases it on activation. Proving
	// the held sector above released its deposit, so this zero base is real.
	req.True(f.preCommitDeposits(t).Equals(big.Zero()), "no precommit deposit may be outstanding here")
	nativePre, err := f.mixed.PreCommitSectors(f.sealProof, kit.NewSectorBatch().AddEmptySectors(1))
	req.NoError(err)
	req.Len(nativePre, 1)
	mN2 := nativePre[0]
	req.True(f.preCommitDeposits(t).GreaterThan(big.Zero()), "a native precommit must reserve a deposit")

	provenN2, err := f.mixed.ProvePrecommittedSectors(f.sealProof, nativePre)
	req.NoError(err)
	req.Equal(nativePre, provenN2, "the native precommit must prove")
	req.True(f.preCommitDeposits(t).Equals(big.Zero()), "activation must release the whole precommit deposit")

	// Native CC on mixed; on deals, a twin of the legacy unverified deal plus two sectors whose
	// manifests point to allocations verifreg won't satisfy.
	nativeCC, _ := f.mixed.OnboardSectors(f.sealProof, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(nativeCC, 1)
	f.mN3 = nativeCC[0]
	dangling := func(alloc verifreg14.AllocationId) kit.SectorManifest {
		return kit.SectorWithVerifiedPiece(f.pieceV.PieceCID,
			&miner14.VerifiedAllocationKey{Client: f.allocV1Client, ID: alloc})
	}
	dNative, _ := f.deals.OnboardSectors(f.sealProof, kit.NewSectorBatch().
		AddSectorsWithRandomPieces(1).
		AddSector(dangling(solsticeUnknownAllocation)).
		AddSector(dangling(verifreg14.AllocationId(f.allocV1))))
	req.Len(dNative, 3, "miner %s must onboard all three native deal sectors", f.deals.ActorAddr)
	dN1 := dNative[0]
	f.dD1, f.dD2 = dNative[1], dNative[2]

	// One wait covers every sector submitted above, since they all gain power at their own first post
	// within the same proving period.
	kit.WaitForMinerQAP(f.ctx, t, f.client, f.mixed.ActorAddr, unit*(9+10+10+10), 5*time.Minute)
	kit.WaitForMinerQAP(f.ctx, t, f.client, f.deals.ActorAddr, unit*(10+10+1+1+1+10+10+10+10), 5*time.Minute)

	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	req.Equal(unit*12, f.client.MinerRawPower(f.ctx, f.mixed.ActorAddr, head.Key()),
		"miner %s holds twelve sectors", f.mixed.ActorAddr)
	req.Equal(unit*9, f.client.MinerRawPower(f.ctx, f.deals.ActorAddr, head.Key()),
		"miner %s holds nine sectors", f.deals.ActorAddr)

	for _, sn := range []abi.SectorNumber{f.mP1, mN2, f.mN3} {
		info := f.sectorInfo(t, f.mixed.ActorAddr, sn, head.Key())
		req.NotZero(info.Flags&miner.FULL_QA_POWER,
			"miner %s sector %d must carry FULL_QA_POWER", f.mixed.ActorAddr, sn)
		req.True(info.DealWeight.NilOrZero(),
			"miner %s sector %d holds no deal, so it must carry no deal weight", f.mixed.ActorAddr, sn)
		req.Positive(info.InitialPledge.Uint64(),
			"miner %s sector %d must carry a pledge", f.mixed.ActorAddr, sn)
	}

	// The pledge of a native sector sits at the FULL-QA tier. The oracle estimates that tier about 10%
	// high, and live reward state drifts from the head-tipset estimate, so the bound is loose.
	n2 := f.sectorInfo(t, f.mixed.ActorAddr, mN2, head.Key())
	duration := n2.Expiration - n2.PowerBaseEpoch
	oneX, err := f.client.StateMinerInitialPledgeForSector(f.ctx, duration, solsticeSectorSize, 0, head.Key())
	req.NoError(err)
	fullQA, err := f.client.StateMinerInitialPledgeForSector(f.ctx, duration, solsticeSectorSize, uint64(solsticeSectorSize), head.Key())
	req.NoError(err)
	req.Greater(fullQA.Uint64(), oneX.Uint64(), "the FULL-QA pledge oracle must exceed the 1x oracle")
	req.Greater(duration, abi.ChainEpoch(0), "miner %s sector %d must have a positive duration", f.mixed.ActorAddr, mN2)
	req.Greater(n2.InitialPledge.Uint64(), oneX.Uint64(),
		"a native sector must be pledged above the 1x tier; on-chain %s, 1x oracle %s", n2.InitialPledge, oneX)
	req.GreaterOrEqual(n2.InitialPledge.Uint64(), fullQA.Uint64()/2,
		"a native sector must be pledged at the FULL-QA tier; on-chain %s, FULL-QA oracle %s", n2.InitialPledge, fullQA)

	_, err = f.client.StateMinerInitialPledgeCollateral(f.ctx, f.mixed.ActorAddr, miner.SectorPreCommitInfo{ //nolint:staticcheck // the deprecated call is the subject
		SealProof: f.sealProof, SectorNumber: mN2, Expiration: n2.Expiration,
	}, head.Key())
	req.ErrorContains(err, "unsupported from network version 29",
		"StateMinerInitialPledgeCollateral for miner %s sector %d must be refused on NV29",
		f.mixed.ActorAddr, mN2)

	// A precommit with a deal is rejected: deals are not activated at precommit.
	dealPrecommit := must.One(actors.SerializeParams(&stminer.PreCommitSectorBatchParams2{
		Sectors: []stminer.SectorPreCommitInfo{{
			SealProof: f.sealProof, SectorNumber: abi.SectorNumber(1 << 20), SealedCID: kit.BogusPieceCid1,
			SealRandEpoch: head.Height() - 1, DealIDs: []abi.DealID{1}, Expiration: head.Height() + abi.ChainEpoch(1<<20),
		}},
	}))
	res, err := f.client.StateCall(f.ctx, &types.Message{
		From: f.mixed.OwnerKey.Address, To: f.mixed.ActorAddr,
		Method: builtin.MethodsMiner.PreCommitSectorBatch2, Params: dealPrecommit, Value: big.Zero(),
	}, head.Key())
	req.NoError(err)
	req.Equal(exitcode.ErrIllegalArgument, res.MsgRct.ExitCode,
		"miner %s: a precommit with a deal must be rejected", f.mixed.ActorAddr)

	// The native deal twin against its legacy twin: same content, different onboarding epoch.
	native := f.sectorInfo(t, f.deals.ActorAddr, dN1, head.Key())
	legacy := f.sectorInfo(t, f.deals.ActorAddr, f.dU1, head.Key())
	req.NotZero(native.Flags&miner.FULL_QA_POWER, "a native deal sector must carry FULL_QA_POWER whatever its content")
	req.Zero(native.DealWeight.Int64(), "FULL_QA zeroes DealWeight on a native deal sector")
	req.NotZero(native.VerifiedDealWeight.Int64(), "FULL_QA holds the whole sector's quality weight in VerifiedDealWeight")
	req.NotZero(legacy.DealWeight.Int64(), "the legacy twin keeps its 1x deal weight")
	req.Zero(legacy.VerifiedDealWeight.Int64(), "the legacy twin holds no verified weight")
	req.NotEqual(legacy.Flags&miner.FULL_QA_POWER, native.Flags&miner.FULL_QA_POWER,
		"content-identical deal sectors must differ in FULL_QA_POWER by onboarding epoch alone")

	// The helper's verdict, summed over every active sector, must be the miner's real QA power.
	f.requireHelperMatchesPower(t, f.deals.ActorAddr, head.Key())
	f.requireHelperMatchesPower(t, f.mixed.ActorAddr, head.Key())

	f.mixed.AssertNoWindowPostError()
	f.deals.AssertNoWindowPostError()
}

// danglingAllocations proves dD1 (an allocation never issued) and dD2 (dV1's already claimed one)
// are live at 10x with the verified registry untouched.
func (f *solsticeLifecycle) danglingAllocations(t *testing.T) {
	req := require.New(t)
	unit := uint64(solsticeSectorSize)

	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)

	active, err := f.client.StateMinerActiveSectors(f.ctx, f.deals.ActorAddr, head.Key())
	req.NoError(err)
	live := make(map[abi.SectorNumber]bool, len(active))
	for _, info := range active {
		live[info.SectorNumber] = true
	}

	for _, sn := range []abi.SectorNumber{f.dD1, f.dD2} {
		req.True(live[sn], "miner %s sector %d must be active", f.deals.ActorAddr, sn)
		info := f.sectorInfo(t, f.deals.ActorAddr, sn, head.Key())
		req.GreaterOrEqual(info.Activation, f.upgradeEpoch,
			"miner %s sector %d must have activated on NV29", f.deals.ActorAddr, sn)
		req.NotZero(info.Flags&miner.FULL_QA_POWER,
			"miner %s sector %d must carry FULL_QA_POWER", f.deals.ActorAddr, sn)
		req.Zero(info.DealWeight.Int64(),
			"miner %s sector %d must carry no deal weight", f.deals.ActorAddr, sn)
		want := big.Mul(big.NewInt(int64(solsticeSectorSize)), big.NewInt(int64(info.Expiration-info.PowerBaseEpoch)))
		req.Equal(want.String(), info.VerifiedDealWeight.String(),
			"miner %s sector %d must hold the whole sector's space over its whole duration",
			f.deals.ActorAddr, sn)
		req.Positive(info.InitialPledge.Uint64(),
			"miner %s sector %d must carry a pledge", f.deals.ActorAddr, sn)
	}

	qap, _ := f.client.MinerQAP(f.ctx, f.deals.ActorAddr, head.Key())
	req.Equal(unit*(10+10+1+1+1+10+10+10+10), qap,
		"miner %s holds nine sectors, sectors %d and %d among them at 10x",
		f.deals.ActorAddr, f.dD1, f.dD2)
	f.requireHelperMatchesPower(t, f.deals.ActorAddr, head.Key())

	unknown, err := f.client.StateGetClaim(f.ctx, f.deals.ActorAddr, verifreg.ClaimId(solsticeUnknownAllocation), head.Key())
	req.NoError(err)
	req.Nil(unknown, "miner %s sector %d must not create claim %d",
		f.deals.ActorAddr, f.dD1, solsticeUnknownAllocation)
	reused, err := f.client.StateGetClaim(f.ctx, f.deals.ActorAddr, verifreg.ClaimId(f.allocV1), head.Key())
	req.NoError(err)
	req.NotNil(reused, "miner %s claim %d must still serve", f.deals.ActorAddr, f.allocV1)
	req.Equal(f.dV1, reused.Sector,
		"miner %s claim %d must still point to sector %d, not %d", f.deals.ActorAddr, f.allocV1, f.dV1, f.dD2)

	claimsWere, err := f.client.StateGetClaims(f.ctx, f.deals.ActorAddr, f.migrated)
	req.NoError(err)
	claimsAre, err := f.client.StateGetClaims(f.ctx, f.deals.ActorAddr, head.Key())
	req.NoError(err)
	req.Equal(claimsWere, claimsAre,
		"miner %s's claims must read as the migration left them", f.deals.ActorAddr)

	allocationsWere, err := f.client.StateGetAllocations(f.ctx, f.verifiedClient, f.migrated)
	req.NoError(err)
	allocationsAre, err := f.client.StateGetAllocations(f.ctx, f.verifiedClient, head.Key())
	req.NoError(err)
	req.Equal(allocationsWere, allocationsAre,
		"client %s's allocations must read as the migration left them", f.verifiedClient)
	req.Equal(f.datacapBalance(t, f.verifiedClient, f.migrated).String(),
		f.datacapBalance(t, f.verifiedClient, head.Key()).String(),
		"client %s's datacap balance must read as the migration left it", f.verifiedClient)

	f.deals.AssertNoWindowPostError()
}

// upgradeQualityCLI upgrades two of six legacy sectors, then the remainder, and checks that
// simulations leave state alone, estimates match the power gained, and a final run does no work.
func (f *solsticeLifecycle) upgradeQualityCLI(t *testing.T, minerCLI *kit.MockCLIClient) {
	req := require.New(t)
	unit := uint64(solsticeSectorSize)
	actorFlag := "--actor=" + f.cli.ActorAddr.String()
	_, err := minerCLI.RunCmdRaw("sectors", "upgrade-quality", actorFlag, "--max-sectors=-1")
	req.ErrorContains(err, "max-sectors must be >= 0")

	sectors := func() map[abi.SectorNumber]*miner.SectorOnChainInfo {
		head, err := f.client.ChainHead(f.ctx)
		req.NoError(err)
		infos := make(map[abi.SectorNumber]*miner.SectorOnChainInfo, len(f.cL))
		for _, sn := range f.cL {
			infos[sn] = f.sectorInfo(t, f.cli.ActorAddr, sn, head.Key())
		}
		return infos
	}
	noPendingUpgrades := func() {
		pending, err := f.client.MpoolPending(f.ctx, types.EmptyTSK)
		req.NoError(err)
		for _, msg := range pending {
			req.False(msg.Message.To == f.cli.ActorAddr && msg.Message.Method == builtin.MethodsMiner.UpgradeSectorQuality,
				"no upgrade message for miner %s should be in the pool", f.cli.ActorAddr)
		}
	}
	estimates := func(out string, count int, current uint64) {
		lines := strings.Split(out, "\n")
		delta := uint64(count) * unit * 9
		req.Contains(lines, fmt.Sprintf("Sector upgrades: %d", count))
		req.Contains(lines, "Current miner QAP: "+types.SizeStr(types.NewInt(current)))
		req.Contains(lines, "Miner QAP after upgrades (estimated): "+types.SizeStr(types.NewInt(current+delta)))
		req.Contains(lines, "QAP increase (estimated): "+types.SizeStr(types.NewInt(delta)))
		req.Contains(lines, "skipped 0 faulty sectors")
		pledgeLine := regexp.MustCompile(`(?m)^Additional pledge \(estimated, excluding gas\): (.+)$`).FindStringSubmatch(out)
		req.Len(pledgeLine, 2, "the additional pledge estimate must be present")
		pledge, err := types.ParseFIL(pledgeLine[1])
		req.NoError(err)
		pledgeAmount := abi.TokenAmount(pledge)
		if count == 0 {
			req.True(pledgeAmount.IsZero(), "a no-op must require no additional pledge")
		} else {
			// Mining advances between simulation and inclusion, so the pledge may change before
			// execution.
			req.True(pledgeAmount.GreaterThan(big.Zero()), "legacy CC upgrades require additional pledge")
		}
	}

	upgraded := 0
	var totalGas int64
	for _, step := range []struct {
		limit []string
		count int
	}{
		{limit: []string{"--max-sectors=2"}, count: 2},
		{count: 4}, // Omitting the limit upgrades every remaining eligible sector.
	} {
		args := append([]string{"sectors", "upgrade-quality", actorFlag}, step.limit...)
		beforeSectors := sectors()
		before, err := f.client.StateMinerPower(f.ctx, f.cli.ActorAddr, types.EmptyTSK)
		req.NoError(err)
		current := unit * uint64(len(f.cL)+9*upgraded)
		req.Equal(current, before.MinerPower.QualityAdjPower.Uint64())

		dryRun, err := minerCLI.RunCmdRaw(args...)
		req.NoError(err)
		t.Logf("upgrade-quality dry run: %s", dryRun)
		req.Contains(dryRun, fmt.Sprintf("will send 1 message(s) for %d sectors", step.count))
		req.Empty(solsticeSentMessages(t, dryRun), "a dry run must report no submitted messages")
		estimates(dryRun, step.count, current)
		noPendingUpgrades()
		req.Equal(beforeSectors, sectors(), "a dry run must leave every sector unchanged")
		req.Equal(current, f.minerQAP(t, f.cli.ActorAddr), "a dry run must not change power")

		out, err := minerCLI.RunCmdRaw(append(args, "--really-do-it")...)
		req.NoError(err)
		t.Logf("upgrade-quality: %s", out)
		req.Contains(out, fmt.Sprintf("sent 1 message(s) upgrading %d sectors", step.count))
		estimates(out, step.count, current)
		sent := solsticeSentMessages(t, out)
		req.Len(sent, 1)
		lookup, err := f.client.StateWaitMsg(f.ctx, sent[0], 2, lapi.LookbackNoLimit, true)
		req.NoError(err)
		req.Equal(exitcode.Ok, lookup.Receipt.ExitCode)
		req.Greater(lookup.Receipt.GasUsed, int64(0))
		req.Less(lookup.Receipt.GasUsed, buildconstants.BlockGasLimit)
		totalGas += lookup.Receipt.GasUsed

		newlyUpgraded := 0
		for sn, after := range sectors() {
			before := beforeSectors[sn]
			if before.Flags&miner.FULL_QA_POWER == 0 && after.Flags&miner.FULL_QA_POWER != 0 {
				newlyUpgraded++
				req.True(after.InitialPledge.GreaterThan(before.InitialPledge), "sector %d must gain pledge", sn)
			} else {
				req.Equal(before, after, "sector %d outside this upgrade must remain unchanged", sn)
			}
		}
		req.Equal(step.count, newlyUpgraded)
		upgraded += newlyUpgraded
		after, err := f.client.StateMinerPower(f.ctx, f.cli.ActorAddr, types.EmptyTSK)
		req.NoError(err)
		gain := types.NewInt(uint64(step.count) * unit * 9)
		req.Equal(current+gain.Uint64(), after.MinerPower.QualityAdjPower.Uint64())
		req.Equal(gain.String(), big.Sub(after.TotalPower.QualityAdjPower, before.TotalPower.QualityAdjPower).String(),
			"network QAP must gain exactly what the miner gained")
	}
	req.Equal(len(f.cL), upgraded)
	req.Less(totalGas, buildconstants.BlockGasLimit)

	finalSectors := sectors()
	for _, flags := range [][]string{nil, {"--really-do-it"}} {
		out, err := minerCLI.RunCmdRaw(append([]string{"sectors", "upgrade-quality", actorFlag}, flags...)...)
		req.NoError(err)
		req.Contains(out, "no active, unexpired sectors need a QA power upgrade")
		req.Empty(solsticeSentMessages(t, out))
		estimates(out, 0, unit*uint64(len(f.cL))*10)
		noPendingUpgrades()
		req.Equal(finalSectors, sectors(), "rerunning the command must leave upgraded sectors unchanged")
		req.Equal(unit*uint64(len(f.cL))*10, f.minerQAP(t, f.cli.ActorAddr))
	}
	f.cli.AssertNoWindowPostError()
}

// solsticeSentLine matches upgrade-quality's "[i/n] <cid>" report of a message it pushed.
var solsticeSentLine = regexp.MustCompile(`^\[\d+/\d+\] (\S+)$`)

// solsticeSentMessages picks the message CIDs out of those lines.
func solsticeSentMessages(t *testing.T, out string) []cid.Cid {
	t.Helper()
	var sent []cid.Cid
	for _, line := range strings.Split(out, "\n") {
		match := solsticeSentLine.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		c, err := cid.Parse(match[1])
		require.NoError(t, err, "parsing a message cid out of %q", line)
		sent = append(sent, c)
	}
	return sent
}

// upgradeSectorQuality raises legacy sectors to full quality-adjusted power, one at a time and in a
// batch, and checks the operations that must leave power alone: a repeat, one on a sector already at
// 10x, and one carrying a new expiration.
func (f *solsticeLifecycle) upgradeSectorQuality(t *testing.T) {
	req := require.New(t)
	unit := uint64(solsticeSectorSize)
	l1, l2, l3, l4, l5, l6 := f.mL[0], f.mL[1], f.mL[2], f.mL[3], f.mL[4], f.mL[5]

	before, networkBefore := f.qapAtHead(t, f.mixed.ActorAddr)

	_, err := f.mixed.UpgradeSectorQuality([]abi.SectorNumber{l1}, nil)
	req.NoError(err)
	upgraded := f.sectorInfo(t, f.mixed.ActorAddr, l1, types.EmptyTSK)
	req.NotZero(upgraded.Flags&miner.FULL_QA_POWER, "an upgraded legacy sector must carry FULL_QA_POWER")
	req.True(upgraded.DealWeight.NilOrZero(), "an upgraded CC sector keeps no deal weight")
	req.Equal(before+unit*9, f.minerQAP(t, f.mixed.ActorAddr), "upgrading one 1x sector must add 9x its raw bytes")

	afterFirst := f.minerQAP(t, f.mixed.ActorAddr)
	_, err = f.mixed.UpgradeSectorQuality([]abi.SectorNumber{l1}, nil)
	req.NoError(err, "upgrading an already-upgraded sector must be accepted")
	req.Equal(afterFirst, f.minerQAP(t, f.mixed.ActorAddr), "a repeated upgrade must not move power")

	native := f.sectorInfo(t, f.mixed.ActorAddr, f.mN3, types.EmptyTSK)
	_, err = f.mixed.UpgradeSectorQuality([]abi.SectorNumber{f.mN3}, nil)
	req.NoError(err, "upgrading a sector created at 10x must be accepted")
	req.Equal(afterFirst, f.minerQAP(t, f.mixed.ActorAddr), "upgrading a native 10x sector must not move power")
	req.NotZero(f.sectorInfo(t, f.mixed.ActorAddr, f.mN3, types.EmptyTSK).Flags&miner.FULL_QA_POWER,
		"the native sector keeps its flag")

	// A new expiration on a sector already at 10x extends it without re-deriving the pledge.
	longer := native.Expiration + abi.ChainEpoch(1000)
	_, err = f.mixed.UpgradeSectorQuality([]abi.SectorNumber{f.mN3}, &longer)
	req.NoError(err)
	extended := f.sectorInfo(t, f.mixed.ActorAddr, f.mN3, types.EmptyTSK)
	req.Equal(longer, extended.Expiration, "the upgrade must advance the expiration it was given")
	req.Equal(native.InitialPledge.String(), extended.InitialPledge.String(), "the pledge must carry forward")
	req.Equal(afterFirst, f.minerQAP(t, f.mixed.ActorAddr), "a new expiration must not move power")

	// The same new-expiration path on a legacy sector: it extends without re-deriving the pledge, and
	// the sector stays at 10x rather than climbing again.
	l1Before := f.sectorInfo(t, f.mixed.ActorAddr, l1, types.EmptyTSK)
	l1Longer := l1Before.Expiration + abi.ChainEpoch(1000)
	_, err = f.mixed.UpgradeSectorQuality([]abi.SectorNumber{l1}, &l1Longer)
	req.NoError(err, "upgrading miner %s sector %d again, with a new expiration, must be accepted", f.mixed.ActorAddr, l1)
	l1After := f.sectorInfo(t, f.mixed.ActorAddr, l1, types.EmptyTSK)
	req.Equal(l1Longer, l1After.Expiration,
		"miner %s sector %d must expire where the upgrade asked", f.mixed.ActorAddr, l1)
	req.Equal(l1Before.InitialPledge.String(), l1After.InitialPledge.String(),
		"miner %s sector %d keeps the pledge it already paid", f.mixed.ActorAddr, l1)
	req.NotZero(l1After.Flags&miner.FULL_QA_POWER,
		"miner %s sector %d stays at 10x rather than climbing again", f.mixed.ActorAddr, l1)
	req.Equal(afterFirst, f.minerQAP(t, f.mixed.ActorAddr),
		"a new expiration on an upgraded legacy sector must not move miner %s's power", f.mixed.ActorAddr)

	// A batch that leaves one sector out, the two here will rise, the third stays where it was.
	beforeBatch, networkBeforeBatch := f.qapAtHead(t, f.mixed.ActorAddr)
	_, err = f.mixed.UpgradeSectorQuality([]abi.SectorNumber{l2, l3}, nil)
	req.NoError(err)
	for _, sn := range []abi.SectorNumber{l2, l3} {
		req.NotZero(f.sectorInfo(t, f.mixed.ActorAddr, sn, types.EmptyTSK).Flags&miner.FULL_QA_POWER,
			"batched sector %d must carry FULL_QA_POWER", sn)
	}
	req.Zero(f.sectorInfo(t, f.mixed.ActorAddr, l4, types.EmptyTSK).Flags&miner.FULL_QA_POWER,
		"sector %d was left out of the batch and must stay at 1x", l4)
	afterBatch, networkAfterBatch := f.qapAtHead(t, f.mixed.ActorAddr)
	req.Equal(beforeBatch+unit*18, afterBatch,
		"miner %s must gain 9x for each of the two sectors in the batch", f.mixed.ActorAddr)
	req.Equal(afterBatch-beforeBatch, networkAfterBatch-networkBeforeBatch,
		"the network's power must move by what miner %s gained", f.mixed.ActorAddr)

	// Upgrading with a new expiration, and extending before upgrading, reach the same end state.
	target := f.sectorInfo(t, f.mixed.ActorAddr, l5, types.EmptyTSK).Expiration + abi.ChainEpoch(2000)
	beforePaths := f.minerQAP(t, f.mixed.ActorAddr)
	_, err = f.mixed.UpgradeSectorQuality([]abi.SectorNumber{l5}, &target)
	req.NoError(err)
	f.mixed.ExtendSectorExpiration(l6, target)
	_, err = f.mixed.UpgradeSectorQuality([]abi.SectorNumber{l6}, nil)
	req.NoError(err)

	pathA := f.sectorInfo(t, f.mixed.ActorAddr, l5, types.EmptyTSK)
	pathB := f.sectorInfo(t, f.mixed.ActorAddr, l6, types.EmptyTSK)
	req.NotZero(pathA.Flags&miner.FULL_QA_POWER, "upgrade with a new expiration must reach 10x")
	req.NotZero(pathB.Flags&miner.FULL_QA_POWER, "extend then upgrade must reach 10x")
	req.Equal(target, pathA.Expiration, "both paths must land on the expiration they were given")
	req.Equal(target, pathB.Expiration, "both paths must land on the expiration they were given")
	req.True(pathA.DealWeight.NilOrZero() && pathB.DealWeight.NilOrZero(), "both are CC sectors")
	req.Equal(beforePaths+unit*18, f.minerQAP(t, f.mixed.ActorAddr), "each path adds 9x, once")

	// An upgraded sector is pledged and charged above the 1x sibling it started level with.
	tenX := f.sectorInfo(t, f.mixed.ActorAddr, l1, types.EmptyTSK)
	oneX := f.sectorInfo(t, f.mixed.ActorAddr, l4, types.EmptyTSK)
	req.Greater(tenX.InitialPledge.Uint64(), oneX.InitialPledge.Uint64(),
		"the upgrade must re-derive a higher pledge than its untouched sibling")
	req.Greater(f.maxTerminationFee(t, unit*10, tenX.InitialPledge).Uint64(),
		f.maxTerminationFee(t, unit, oneX.InitialPledge).Uint64(),
		"a 10x sector's maximum termination fee must exceed its 1x sibling's")

	after, networkAfter := f.qapAtHead(t, f.mixed.ActorAddr)
	req.Equal(before+unit*45, after, "miner %s upgraded five sectors, 9x each and nothing else", f.mixed.ActorAddr)
	req.Equal(after-before, networkAfter-networkBefore,
		"the network's power must track miner %s", f.mixed.ActorAddr)
	f.mixed.AssertNoWindowPostError()
}

// extendSectors extends one sector of each tier. An extension moves the expiration and re-derives the
// weights for the new duration, so what it must leave alone is the sector's tier and the miner's
// power. The weights follow the actor's arithmetic: with B the old power base epoch, E the old
// expiration, e the epoch the extension executes and E' the new expiration,
//
//	V' = floor(V/(E-B)) * (E'-e)   the claim's space carried over the new duration
//	D' = floor(D*(E-e)/(E-B))      what is left of the old deal, which does not extend
//
// and the power base epoch moves to e.
func (f *solsticeLifecycle) extendSectors(t *testing.T) {
	req := require.New(t)

	type subject struct {
		name   string
		miner  *kit.TestUnmanagedMiner
		sector abi.SectorNumber
		claims []verifreg14.ClaimId // used in SectorsWithClaims; ESE2 consults no verifreg and ignores it
	}
	subjects := []subject{
		{name: "legacy CC at 1x", miner: f.mixed, sector: f.mL[3]},
		{name: "native CC at 10x", miner: f.mixed, sector: f.mN3},
		{name: "legacy verified deal already extended on NV28", miner: f.deals, sector: f.dV2,
			claims: []verifreg14.ClaimId{verifreg14.ClaimId(f.allocV2)}},
		{name: "legacy unverified deal", miner: f.deals, sector: f.dU1},
	}

	for _, sub := range subjects {
		head, err := f.client.ChainHead(f.ctx)
		req.NoError(err)
		before := f.sectorInfo(t, sub.miner.ActorAddr, sub.sector, head.Key())
		powerBefore := f.minerQAP(t, sub.miner.ActorAddr)
		if sub.sector == f.dV2 {
			req.Greater(before.PowerBaseEpoch, before.Activation,
				"a verified sector extended on NV28 must have a moved power base epoch before its NV29 extension")
		}

		target := before.Expiration + builtin.EpochsInDay
		sub.miner.ExtendSectorExpiration(sub.sector, target, sub.claims...)

		head, err = f.client.ChainHead(f.ctx)
		req.NoError(err)
		after := f.sectorInfo(t, sub.miner.ActorAddr, sub.sector, head.Key())

		req.Equal(target, after.Expiration,
			"%s: miner %s sector %d must expire where the extension asked",
			sub.name, sub.miner.ActorAddr, sub.sector)
		req.Equal(before.Flags, after.Flags,
			"%s: an extension must not change miner %s sector %d's flags", sub.name, sub.miner.ActorAddr, sub.sector)
		req.Equal(before.InitialPledge.String(), after.InitialPledge.String(),
			"%s: an extension must preserve miner %s sector %d's initial pledge",
			sub.name, sub.miner.ActorAddr, sub.sector)
		req.Greater(after.PowerBaseEpoch, before.PowerBaseEpoch,
			"%s: an extension must move miner %s sector %d's power base epoch to the epoch it ran",
			sub.name, sub.miner.ActorAddr, sub.sector)
		req.LessOrEqual(after.PowerBaseEpoch, head.Height(),
			"%s: miner %s sector %d's power base epoch cannot be in the future",
			sub.name, sub.miner.ActorAddr, sub.sector)

		executedAt := after.PowerBaseEpoch
		oldDuration := big.NewInt(int64(before.Expiration - before.PowerBaseEpoch))
		newDuration := big.NewInt(int64(after.Expiration - executedAt))
		wantVerified := big.Mul(big.Div(before.VerifiedDealWeight, oldDuration), newDuration)
		wantDeal := big.Div(big.Mul(before.DealWeight, big.NewInt(int64(before.Expiration-executedAt))), oldDuration)
		req.Equal(wantVerified.String(), after.VerifiedDealWeight.String(),
			"%s: miner %s sector %d's verified weight must be its claim's space over the new duration",
			sub.name, sub.miner.ActorAddr, sub.sector)
		req.Equal(wantDeal.String(), after.DealWeight.String(),
			"%s: miner %s sector %d's deal weight must be what is left of the old deal",
			sub.name, sub.miner.ActorAddr, sub.sector)
		req.Equal(before.VerifiedDealWeight.Sign(), after.VerifiedDealWeight.Sign(),
			"%s: an extension must neither add nor drop miner %s sector %d's claim",
			sub.name, sub.miner.ActorAddr, sub.sector)
		req.Equal(powerBefore, f.minerQAP(t, sub.miner.ActorAddr),
			"%s: an extension must not move miner %s's power", sub.name, sub.miner.ActorAddr)
	}

	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	l4 := f.sectorInfo(t, f.mixed.ActorAddr, f.mL[3], head.Key())
	req.Zero(l4.Flags&miner.FULL_QA_POWER,
		"extending miner %s sector %d must not promote it", f.mixed.ActorAddr, f.mL[3])
	req.Zero(l4.DealWeight.Int64(),
		"miner %s sector %d holds no deal, so it must not gain deal weight", f.mixed.ActorAddr, f.mL[3])
	req.Zero(l4.VerifiedDealWeight.Int64(),
		"miner %s sector %d holds no claim, so it must not gain verified weight", f.mixed.ActorAddr, f.mL[3])
	req.Zero(f.sectorInfo(t, f.deals.ActorAddr, f.dU1, head.Key()).Flags&miner.FULL_QA_POWER,
		"extending miner %s sector %d must not promote it", f.deals.ActorAddr, f.dU1)
	req.Positive(f.sectorInfo(t, f.deals.ActorAddr, f.dV2, head.Key()).VerifiedDealWeight.Int64(),
		"extending miner %s sector %d must keep its claim", f.deals.ActorAddr, f.dV2)
	f.mixed.AssertNoWindowPostError()
	f.deals.AssertNoWindowPostError()
}

// snapOrdering snaps and upgrades the same pair of sectors in both orders, and snaps a sector already
// at 10x. Each sector must end at 10x having gained it exactly once.
func (f *solsticeLifecycle) snapOrdering(t *testing.T) {
	req := require.New(t)
	unit := uint64(solsticeSectorSize)
	l7, l8 := f.mL[6], f.mL[7]

	before := f.minerQAP(t, f.mixed.ActorAddr)

	_, err := f.mixed.UpgradeSectorQuality([]abi.SectorNumber{l7}, nil)
	req.NoError(err)
	f.mixed.SnapDeal(l7, kit.SectorWithPiece(kit.BogusPieceCid2))

	f.mixed.SnapDeal(l8, kit.SectorWithPiece(kit.BogusPieceCid2))
	_, err = f.mixed.UpgradeSectorQuality([]abi.SectorNumber{l8}, nil)
	req.NoError(err)

	for _, sn := range []abi.SectorNumber{l7, l8} {
		req.NotZero(f.sectorInfo(t, f.mixed.ActorAddr, sn, types.EmptyTSK).Flags&miner.FULL_QA_POWER,
			"sector %d must carry FULL_QA_POWER whichever order it went through", sn)
	}
	req.Equal(before+unit*18, f.minerQAP(t, f.mixed.ActorAddr),
		"either order adds 9x per sector, with nothing counted twice")

	beforeNative := f.minerQAP(t, f.mixed.ActorAddr)
	f.mixed.SnapDeal(f.mN3, kit.SectorWithPiece(kit.BogusPieceCid2))
	req.NotZero(f.sectorInfo(t, f.mixed.ActorAddr, f.mN3, types.EmptyTSK).Flags&miner.FULL_QA_POWER,
		"snapping a native 10x sector must keep its flag")
	req.Equal(beforeNative, f.minerQAP(t, f.mixed.ActorAddr),
		"snapping a sector already at 10x must not move power")
	f.mixed.AssertNoWindowPostError()
}

// dealTiers upgrades the deal sectors: the unverified one rises, the verified one is already at 10x
// through its claim and has nothing to gain.
func (f *solsticeLifecycle) dealTiers(t *testing.T) {
	req := require.New(t)
	unit := uint64(solsticeSectorSize)

	before := f.minerQAP(t, f.deals.ActorAddr)
	_, err := f.deals.UpgradeSectorQuality([]abi.SectorNumber{f.dU1}, nil)
	req.NoError(err)
	req.NotZero(f.sectorInfo(t, f.deals.ActorAddr, f.dU1, types.EmptyTSK).Flags&miner.FULL_QA_POWER,
		"upgrading an unverified deal sector must set its flag")
	req.Equal(before+unit*9, f.minerQAP(t, f.deals.ActorAddr),
		"an unverified deal sector must rise from 1x to 10x")

	afterUnverified := f.minerQAP(t, f.deals.ActorAddr)
	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	verifiedBefore := f.sectorInfo(t, f.deals.ActorAddr, f.dV1, head.Key())
	req.Zero(verifiedBefore.Flags&miner.FULL_QA_POWER,
		"miner %s sector %d is at 10x through its weight, not a flag", f.deals.ActorAddr, f.dV1)

	_, err = f.deals.UpgradeSectorQuality([]abi.SectorNumber{f.dV1}, nil)
	req.NoError(err, "upgrading miner %s sector %d must be accepted", f.deals.ActorAddr, f.dV1)

	head, err = f.client.ChainHead(f.ctx)
	req.NoError(err)
	verifiedAfter := f.sectorInfo(t, f.deals.ActorAddr, f.dV1, head.Key())
	req.Zero(verifiedAfter.Flags&miner.FULL_QA_POWER,
		"miner %s sector %d is already at 10x, so there is nothing to upgrade", f.deals.ActorAddr, f.dV1)
	req.Equal(verifiedBefore.VerifiedDealWeight.String(), verifiedAfter.VerifiedDealWeight.String(),
		"miner %s sector %d keeps the weight its content earned", f.deals.ActorAddr, f.dV1)
	req.Equal(afterUnverified, f.minerQAP(t, f.deals.ActorAddr),
		"upgrading a sector already at 10x must not move miner %s's power", f.deals.ActorAddr)

	req.GreaterOrEqual(verifiedAfter.PowerBaseEpoch, verifiedAfter.Activation,
		"miner %s sector %d cannot have a power base epoch before its activation", f.deals.ActorAddr, f.dV1)
	f.requireHelperMatchesPower(t, f.deals.ActorAddr, head.Key())
	f.deals.AssertNoWindowPostError()
}

// deadlineImmutability walks one sector's deadline through the window either side of its own, where
// the actor refuses to change the sectors it's about to challenge. Termination is refused there and
// accepted outside it; a quality upgrade is accepted throughout.
func (f *solsticeLifecycle) deadlineImmutability(t *testing.T) {
	req := require.New(t)

	probed, sn := f.soonestDeadlineSector(t)
	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	loc, err := f.client.StateSectorPartition(f.ctx, probed.ActorAddr, sn, head.Key())
	req.NoError(err)

	// The upgrade probe only says anything if the sector still has something to gain: on a sector
	// already at 10x the actor returns Ok without touching it, whatever window it is in.
	subject := f.sectorInfo(t, probed.ActorAddr, sn, head.Key())
	req.False(miner.SectorIsFullQaPower(subject),
		"miner %s sector %d must still be at 1x for this probe to mean anything", probed.ActorAddr, sn)
	active, err := f.client.StateMinerActiveSectors(f.ctx, probed.ActorAddr, head.Key())
	req.NoError(err)
	isActive := false
	for _, info := range active {
		isActive = isActive || info.SectorNumber == sn
	}
	req.True(isActive, "miner %s sector %d must be active for this probe", probed.ActorAddr, sn)

	terminate := must.One(actors.SerializeParams(&stminer.TerminateSectorsParams{
		Terminations: []stminer.TerminationDeclaration{{
			Deadline: loc.Deadline, Partition: loc.Partition, Sectors: bitfield.NewFromSet([]uint64{uint64(sn)}),
		}},
	}))
	upgrade := f.client.UpgradeSectorQualityParams(f.ctx, probed.ActorAddr, sn, head.Key())

	probe := func(tsk types.TipSetKey, method abi.MethodNum, params []byte) exitcode.ExitCode {
		res, err := f.client.StateCall(f.ctx, &types.Message{
			From: probed.OwnerKey.Address, To: probed.ActorAddr, Method: method,
			Params: params, Value: big.Zero(),
		}, tsk)
		req.NoError(err)
		return res.MsgRct.ExitCode
	}

	nd := miner.WPoStPeriodDeadlines
	for _, position := range []struct {
		name  string
		index uint64
		term  exitcode.ExitCode
	}{
		{name: "the window before the sector's", index: (loc.Deadline + nd - 1) % nd, term: exitcode.ErrIllegalArgument},
		{name: "the sector's own window", index: loc.Deadline, term: exitcode.ErrIllegalArgument},
		{name: "the window after the sector's", index: (loc.Deadline + 1) % nd, term: exitcode.Ok},
	} {
		tsk := f.client.WaitForDeadlineIndex(f.ctx, probed.ActorAddr, position.index)
		req.Equal(position.term, probe(tsk, builtin.MethodsMiner.TerminateSectors, terminate),
			"terminating miner %s sector %d in %s", probed.ActorAddr, sn, position.name)
		req.Equal(exitcode.Ok, probe(tsk, builtin.MethodsMiner.UpgradeSectorQuality, upgrade),
			"a quality upgrade is never gated on the deadline: miner %s sector %d in %s",
			probed.ActorAddr, sn, position.name)
	}
	probed.AssertNoWindowPostError()
}

// upgradeAuthorization checks who may raise a sector's quality: the owner and a control address, and
// nobody else.
func (f *solsticeLifecycle) upgradeAuthorization(t *testing.T) {
	req := require.New(t)
	unit := uint64(solsticeSectorSize)
	l9 := f.mL[8]

	control, err := f.client.WalletNew(f.ctx, types.KTSecp256k1)
	req.NoError(err)
	unrelated, err := f.client.WalletNew(f.ctx, types.KTSecp256k1)
	req.NoError(err)
	for _, a := range []address.Address{control, unrelated} {
		kit.SendFunds(f.ctx, t, f.client, a, types.FromFil(1))
	}

	info, err := f.client.StateMinerInfo(f.ctx, f.mixed.ActorAddr, types.EmptyTSK)
	req.NoError(err)
	changed := must.One(actors.SerializeParams(&stminer.ChangeWorkerAddressParams{
		NewWorker:       info.Worker, // unchanged, so the control addresses take effect at once
		NewControlAddrs: []address.Address{control},
	}))
	msg, err := f.client.MpoolPushMessage(f.ctx, &types.Message{
		From: f.mixed.OwnerKey.Address, To: f.mixed.ActorAddr,
		Method: builtin.MethodsMiner.ChangeWorkerAddress, Params: changed, Value: big.Zero(),
	}, nil)
	req.NoError(err)
	lookup, err := f.client.StateWaitMsg(f.ctx, msg.Cid(), 2, lapi.LookbackNoLimit, true)
	req.NoError(err)
	req.Equal(exitcode.Ok, lookup.Receipt.ExitCode, "installing a control address")

	controlID, err := f.client.StateLookupID(f.ctx, control, lookup.TipSet)
	req.NoError(err)
	ownerID, err := f.client.StateLookupID(f.ctx, f.mixed.OwnerKey.Address, lookup.TipSet)
	req.NoError(err)
	info, err = f.client.StateMinerInfo(f.ctx, f.mixed.ActorAddr, lookup.TipSet)
	req.NoError(err)
	req.Contains(info.ControlAddresses, controlID, "miner %s must have the control address", f.mixed.ActorAddr)
	req.Equal(ownerID, info.Owner, "miner %s keeps its owner through a control-address change", f.mixed.ActorAddr)
	req.Equal(ownerID, info.Worker, "miner %s keeps its worker through a control-address change", f.mixed.ActorAddr)

	upgrade := f.client.UpgradeSectorQualityParams(f.ctx, f.mixed.ActorAddr, l9, lookup.TipSet)
	callExit := func(from address.Address) exitcode.ExitCode {
		res, err := f.client.StateCall(f.ctx, &types.Message{
			From: from, To: f.mixed.ActorAddr, Method: builtin.MethodsMiner.UpgradeSectorQuality,
			Params: upgrade, Value: big.Zero(),
		}, types.EmptyTSK)
		req.NoError(err)
		return res.MsgRct.ExitCode
	}
	req.Equal(exitcode.Ok, callExit(control), "a control address may upgrade sector quality")
	req.Equal(exitcode.ErrForbidden, callExit(unrelated), "an unrelated address may not")

	before := f.minerQAP(t, f.mixed.ActorAddr)
	_, err = f.mixed.UpgradeSectorQuality([]abi.SectorNumber{l9}, nil)
	req.NoError(err, "the owner may upgrade sector quality")
	req.NotZero(f.sectorInfo(t, f.mixed.ActorAddr, l9, types.EmptyTSK).Flags&miner.FULL_QA_POWER,
		"the owner's upgrade must actually raise the sector")
	req.Equal(before+unit*9, f.minerQAP(t, f.mixed.ActorAddr), "and it must be worth 9x more")
	f.mixed.AssertNoWindowPostError()
}

// terminations removes one sector of each tier and checks the miner is left with exactly the power of
// what remains, counted from the ledger rather than from what was observed a moment ago.
func (f *solsticeLifecycle) terminations(t *testing.T) {
	req := require.New(t)
	unit := uint64(solsticeSectorSize)

	for _, step := range []struct {
		name   string
		miner  *kit.TestUnmanagedMiner
		sector abi.SectorNumber
		tier   uint64
		left   uint64 // what the miner's remaining sectors are worth, in sector-size units
	}{
		{name: "an unverified deal sector at 1x", miner: f.deals, sector: f.dU2, tier: 1, left: 71},
		{name: "a verified deal sector at 10x", miner: f.deals, sector: f.dV1, tier: 10, left: 61},
		{name: "a CC sector at 1x", miner: f.mixed, sector: f.mL[3], tier: 1, left: 110},
		{name: "a CC sector at 10x", miner: f.mixed, sector: f.mL[0], tier: 10, left: 100},
	} {
		before := f.minerQAP(t, step.miner.ActorAddr)
		want := unit * step.left
		req.Equal(before, want+unit*step.tier,
			"miner %s must hold the sector about to go and %d units besides", step.miner.ActorAddr, step.left)

		step.miner.TerminateSectors([]abi.SectorNumber{step.sector})
		kit.WaitForMinerQAP(f.ctx, t, f.client, step.miner.ActorAddr, want, 3*time.Minute)

		head, err := f.client.ChainHead(f.ctx)
		req.NoError(err)
		after, _ := f.client.MinerQAP(f.ctx, step.miner.ActorAddr, head.Key())
		req.Equal(want, after, "terminating %s must leave miner %s with %d units",
			step.name, step.miner.ActorAddr, step.left)
		req.Equal(unit*step.tier, before-after, "terminating %s must remove its own tier and no more", step.name)

		// The sector stops counting at once. Its record stays until the miner compacts its
		// partitions, which nothing here does.
		active, err := f.client.StateMinerActiveSectors(f.ctx, step.miner.ActorAddr, head.Key())
		req.NoError(err)
		for _, info := range active {
			req.NotEqual(step.sector, info.SectorNumber,
				"terminating %s must take miner %s sector %d out of the active set",
				step.name, step.miner.ActorAddr, step.sector)
		}
		record, err := f.client.StateSectorGetInfo(f.ctx, step.miner.ActorAddr, step.sector, head.Key())
		req.NoError(err)
		req.NotNil(record, "miner %s sector %d keeps its record until the partition is compacted",
			step.miner.ActorAddr, step.sector)
	}

	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	f.requireHelperMatchesPower(t, f.mixed.ActorAddr, head.Key())
	f.requireHelperMatchesPower(t, f.deals.ActorAddr, head.Key())
	f.mixed.AssertNoWindowPostError()
	f.deals.AssertNoWindowPostError()
}

// soonestDeadlineSector picks, among the sectors that are still at 1x, the one whose deadline comes
// round first, so the walk through the windows either side of it is as short as the assignment allows.
func (f *solsticeLifecycle) soonestDeadlineSector(t *testing.T) (*kit.TestUnmanagedMiner, abi.SectorNumber) {
	req := require.New(t)

	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)

	var (
		bestMiner  *kit.TestUnmanagedMiner
		bestSector abi.SectorNumber
		bestWait   uint64
	)
	for _, candidate := range []struct {
		m      *kit.TestUnmanagedMiner
		sector abi.SectorNumber
	}{
		{f.mixed, f.mL[3]},
		{f.deals, f.dU2},
		{f.deals, f.dC1},
	} {
		info := f.sectorInfo(t, candidate.m.ActorAddr, candidate.sector, head.Key())
		req.False(miner.SectorIsFullQaPower(info),
			"miner %s sector %d is meant to stay at 1x", candidate.m.ActorAddr, candidate.sector)

		di := kit.DeadlineForHeight(must.One(f.client.StateMinerProvingDeadline(f.ctx, candidate.m.ActorAddr, head.Key())), head.Height())
		loc, err := f.client.StateSectorPartition(f.ctx, candidate.m.ActorAddr, candidate.sector, head.Key())
		req.NoError(err)
		// The walk starts one window before the sector's own.
		wait := (loc.Deadline + di.WPoStPeriodDeadlines - 1 - di.Index) % di.WPoStPeriodDeadlines
		if bestMiner == nil || wait < bestWait {
			bestMiner, bestSector, bestWait = candidate.m, candidate.sector, wait
		}
	}
	t.Logf("probing miner %s sector %d, whose deadline comes round in %d windows",
		bestMiner.ActorAddr, bestSector, bestWait)
	return bestMiner, bestSector
}

// maxTerminationFee asks the miner actor what it would charge to terminate a sector of this power and
// pledge.
func (f *solsticeLifecycle) maxTerminationFee(t *testing.T, power uint64, pledge abi.TokenAmount) abi.TokenAmount {
	t.Helper()
	req := require.New(t)

	params := must.One(actors.SerializeParams(&miner.MaxTerminationFeeParams{
		Power: types.NewInt(power), InitialPledge: pledge,
	}))
	msg, err := f.client.MpoolPushMessage(f.ctx, &types.Message{
		From: f.mixed.OwnerKey.Address, To: f.mixed.ActorAddr,
		Method: builtin.MethodsMiner.MaxTerminationFeeExported, Params: params, Value: big.Zero(),
	}, nil)
	req.NoError(err)
	lookup, err := f.client.StateWaitMsg(f.ctx, msg.Cid(), 1, lapi.LookbackNoLimit, true)
	req.NoError(err)
	req.Equal(exitcode.Ok, lookup.Receipt.ExitCode, "MaxTerminationFeeExported on miner %s", f.mixed.ActorAddr)

	var fee miner.MaxTerminationFeeReturn
	req.NoError(fee.UnmarshalCBOR(bytes.NewReader(lookup.Receipt.Return)))
	return fee
}

// datacapBalance reads an address's datacap token balance through a StateCall.
func (f *solsticeLifecycle) datacapBalance(t *testing.T, addr address.Address, tsk types.TipSetKey) abi.TokenAmount {
	t.Helper()
	req := require.New(t)

	params := must.One(actors.SerializeParams(&addr))
	res, err := f.client.StateCall(f.ctx, &types.Message{
		From: f.mixed.OwnerKey.Address, To: datacap.Address,
		Method: datacap.Methods.BalanceExported, Params: params, Value: big.Zero(),
	}, tsk)
	req.NoError(err)
	req.Equal(exitcode.Ok, res.MsgRct.ExitCode, "datacap Balance for %s", addr)

	var balance abi.TokenAmount
	req.NoError(balance.UnmarshalCBOR(bytes.NewReader(res.MsgRct.Return)))
	return balance
}

// qapAtHead reads the miner's and the network's quality-adjusted power together, at the head, so the
// two are never taken from different tipsets.
func (f *solsticeLifecycle) qapAtHead(t *testing.T, maddr address.Address) (minerQAP, networkQAP uint64) {
	t.Helper()
	head, err := f.client.ChainHead(f.ctx)
	require.NoError(t, err)
	return f.client.MinerQAP(f.ctx, maddr, head.Key())
}

// minerQAP reads one miner's quality-adjusted power at the head.
func (f *solsticeLifecycle) minerQAP(t *testing.T, maddr address.Address) uint64 {
	t.Helper()
	qap, _ := f.qapAtHead(t, maddr)
	return qap
}

// requireHelperMatchesPower checks miner.SectorIsFullQaPower against the miner's real QA power: every
// sector here is a full-size 2KiB sector, so each contributes exactly 1x or 10x.
func (f *solsticeLifecycle) requireHelperMatchesPower(t *testing.T, maddr address.Address, tsk types.TipSetKey) {
	req := require.New(t)

	active, err := f.client.StateMinerActiveSectors(f.ctx, maddr, tsk)
	req.NoError(err)
	minerQAP, _ := f.client.MinerQAP(f.ctx, maddr, tsk)

	sum := uint64(0)
	for _, info := range active {
		req.Greater(info.Expiration, info.PowerBaseEpoch,
			"miner %s sector %d must have a positive duration", maddr, info.SectorNumber)
		mult := uint64(1)
		if miner.SectorIsFullQaPower(info) {
			mult = 10
		}
		sum += uint64(solsticeSectorSize) * mult
	}
	req.Equal(minerQAP, sum,
		"miner %s: SectorIsFullQaPower classification must match on-chain QA power", maddr)
}

func (f *solsticeLifecycle) sectorsOf(t *testing.T, m *kit.TestUnmanagedMiner) []abi.SectorNumber {
	switch m {
	case f.mixed:
		return f.mL
	case f.deals:
		return []abi.SectorNumber{f.dV1, f.dV2, f.dU1, f.dU2, f.dC1, f.dC2}
	case f.cli:
		return f.cL
	}
	require.FailNow(t, "no sectors recorded for this miner", "miner %s", m.ActorAddr)
	return nil
}

func (f *solsticeLifecycle) sectorInfo(t *testing.T, maddr address.Address, sn abi.SectorNumber, tsk types.TipSetKey) *miner.SectorOnChainInfo {
	t.Helper()
	return f.client.MustSectorInfo(f.ctx, maddr, sn, tsk)
}

// preCommitDeposits reads what the mixed miner has reserved against unproven precommits.
func (f *solsticeLifecycle) preCommitDeposits(t *testing.T) abi.TokenAmount {
	t.Helper()
	return f.client.MinerState(f.ctx, f.mixed.ActorAddr, types.EmptyTSK).PreCommitDeposits
}

// requireSolsticeFrozenActors checks FIP-0118's refusal at every mutating entry point of the datacap
// and verified registry actors, at one migrated tipset. Read-only methods are untouched by the FIP.
func requireSolsticeFrozenActors(ctx context.Context, t *testing.T, client lapi.FullNode, tsk types.TipSetKey) {
	t.Helper()
	from, err := client.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	var (
		verifregAddr = builtin.VerifiedRegistryActorAddr
		datacapAddr  = builtin.DatacapActorAddr
		amount       = big.Mul(big.NewInt(1<<30), builtin.TokenPrecision)
		allowance    = big.NewInt(1 << 30)
		// well-formed enough to deserialize; the actor never gets as far as verifying it
		signature = crypto.Signature{Type: crypto.SigTypeSecp256k1, Data: make([]byte, 65)}
		// FRC-46 token receiver hook payload type, per the frc46_token library
		frc46 = verifreg19.ReceiverType(builtin.MustGenerateFRCMethodNum("FRC46"))
	)

	type frozenMethod struct {
		name   string
		to     address.Address
		method abi.MethodNum
		params cbor.Marshaler
		// callerGated methods reject an account sender at caller validation, ahead of the
		// FIP-0118 refusal, so only the exit code is observable from a plain wallet.
		callerGated bool
	}

	cases := []frozenMethod{
		{
			name: "verifreg/AddVerifier", to: verifregAddr, method: verifreg.Methods.AddVerifier,
			params: &verifreg19.AddVerifierParams{Address: from, Allowance: allowance},
		}, {
			name: "verifreg/RemoveVerifier", to: verifregAddr, method: verifreg.Methods.RemoveVerifier,
			params: &from,
		}, {
			name: "verifreg/AddVerifiedClient", to: verifregAddr, method: verifreg.Methods.AddVerifiedClient,
			params: &verifreg19.AddVerifiedClientParams{Address: from, Allowance: allowance},
		}, {
			name: "verifreg/AddVerifiedClientExported", to: verifregAddr, method: verifreg.Methods.AddVerifiedClientExported,
			params: &verifreg19.AddVerifiedClientParams{Address: from, Allowance: allowance},
		}, {
			name: "verifreg/RemoveVerifiedClientDataCap", to: verifregAddr, method: verifreg.Methods.RemoveVerifiedClientDataCap,
			params: &verifreg19.RemoveDataCapParams{
				VerifiedClientToRemove: from,
				DataCapAmountToRemove:  allowance,
				VerifierRequest1:       verifreg19.RemoveDataCapRequest{Verifier: from, VerifierSignature: signature},
				VerifierRequest2:       verifreg19.RemoveDataCapRequest{Verifier: from, VerifierSignature: signature},
			},
		}, {
			name: "verifreg/RemoveExpiredAllocations", to: verifregAddr, method: verifreg.Methods.RemoveExpiredAllocations,
			params: &verifreg19.RemoveExpiredAllocationsParams{Client: abi.ActorID(100)},
		}, {
			name: "verifreg/RemoveExpiredAllocationsExported", to: verifregAddr, method: verifreg.Methods.RemoveExpiredAllocationsExported,
			params: &verifreg19.RemoveExpiredAllocationsParams{Client: abi.ActorID(100)},
		}, {
			name: "verifreg/ClaimAllocations", to: verifregAddr, method: verifreg.Methods.ClaimAllocations,
			params: &verifreg19.ClaimAllocationsParams{AllOrNothing: true}, callerGated: true,
		}, {
			name: "verifreg/ExtendClaimTerms", to: verifregAddr, method: verifreg.Methods.ExtendClaimTerms,
			params: &verifreg19.ExtendClaimTermsParams{},
		}, {
			name: "verifreg/ExtendClaimTermsExported", to: verifregAddr, method: verifreg.Methods.ExtendClaimTermsExported,
			params: &verifreg19.ExtendClaimTermsParams{},
		}, {
			name: "verifreg/RemoveExpiredClaims", to: verifregAddr, method: verifreg.Methods.RemoveExpiredClaims,
			params: &verifreg19.RemoveExpiredClaimsParams{Provider: abi.ActorID(1000)},
		}, {
			name: "verifreg/RemoveExpiredClaimsExported", to: verifregAddr, method: verifreg.Methods.RemoveExpiredClaimsExported,
			params: &verifreg19.RemoveExpiredClaimsParams{Provider: abi.ActorID(1000)},
		}, {
			// the allocation path: datacap tokens transferred to verifreg land here
			name: "verifreg/UniversalReceiverHook", to: verifregAddr, method: verifreg.Methods.UniversalReceiverHook,
			params: &verifreg19.UniversalReceiverParams{Type_: frc46}, callerGated: true,
		},

		{
			name: "datacap/Mint", to: datacapAddr, method: datacap.Methods.MintExported,
			params: &datacap19.MintParams{To: from, Amount: amount},
		}, {
			name: "datacap/Destroy", to: datacapAddr, method: datacap.Methods.DestroyExported,
			params: &datacap19.DestroyParams{Owner: from, Amount: amount},
		}, {
			name: "datacap/Transfer", to: datacapAddr, method: datacap.Methods.TransferExported,
			params: &datacap19.TransferParams{To: verifregAddr, Amount: amount},
		}, {
			name: "datacap/TransferFrom", to: datacapAddr, method: datacap.Methods.TransferFromExported,
			params: &datacap19.TransferFromParams{From: from, To: verifregAddr, Amount: amount},
		}, {
			name: "datacap/IncreaseAllowance", to: datacapAddr, method: datacap.Methods.IncreaseAllowanceExported,
			params: &datacap19.IncreaseAllowanceParams{Operator: from, Increase: amount},
		}, {
			name: "datacap/DecreaseAllowance", to: datacapAddr, method: datacap.Methods.DecreaseAllowanceExported,
			params: &datacap19.DecreaseAllowanceParams{Operator: from, Decrease: amount},
		}, {
			name: "datacap/RevokeAllowance", to: datacapAddr, method: datacap.Methods.RevokeAllowanceExported,
			params: &datacap19.RevokeAllowanceParams{Operator: from},
		}, {
			name: "datacap/Burn", to: datacapAddr, method: datacap.Methods.BurnExported,
			params: &datacap19.BurnParams{Amount: amount},
		}, {
			name: "datacap/BurnFrom", to: datacapAddr, method: datacap.Methods.BurnFromExported,
			params: &datacap19.BurnFromParams{Owner: from, Amount: amount},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params, aerr := actors.SerializeParams(tc.params)
			require.NoError(t, aerr)

			res, err := client.StateCall(ctx, &types.Message{
				From:   from,
				To:     tc.to,
				Method: tc.method,
				Params: params,
				Value:  big.Zero(),
			}, tsk)
			require.NoError(t, err)

			require.Equal(t, exitcode.ErrForbidden, res.MsgRct.ExitCode, "error was: %s", res.Error)
			if !tc.callerGated {
				require.Contains(t, res.Error, "FIP-0118")
			}
		})
	}
}

// TestSolsticeDailyFee checks the fee a sector pays per day: sectors of the same tier pay the same,
// a 10x sector pays more than a 1x one, an upgrade re-derives the fee, and the partitions' power adds
// up to the miner's.
func TestSolsticeDailyFee(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	// Daily fees are a share of circulating supply, so the reserve is raised to leave a supply that
	// makes them non-zero, as the daily fee itests do.
	originalReserve := buildconstants.UpgradeTeepInitialFilReserved
	buildconstants.UpgradeTeepInitialFilReserved = types.MustParseFIL("1000000000 FIL").Int
	t.Cleanup(func() { buildconstants.UpgradeTeepInitialFilReserved = originalReserve })

	upgradeEpoch := miner14.ChainFinality + miner.WPoStProvingPeriod() + 12*miner.WPoStChallengeWindow()
	e := kit.NewSolsticeUpgradeEnv(t, kit.SolsticeOpts{UpgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	defer um.Stop()
	unit := uint64(solsticeSectorSize)

	legacy, _ := um.OnboardSectors(e.SealProof, kit.NewSectorBatch().AddEmptySectors(2))
	req.Len(legacy, 2)
	req.NoError(um.WaitTillActivatedAndAssertPower(legacy, unit*2, unit*2))

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	nv, err := client.StateNetworkVersion(ctx, types.EmptyTSK)
	req.NoError(err)
	req.Equal(network.Version29, nv, "the chain must be on NV29 after the migration")

	native, _ := um.OnboardSectors(e.SealProof, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(native, 1)
	req.NoError(um.WaitTillActivatedAndAssertPower(native, unit*3, unit*(1+1+10)))

	dailyFee := func(sn abi.SectorNumber, tsk types.TipSetKey) abi.TokenAmount {
		return client.MustSectorInfo(ctx, maddr, sn, tsk).DailyFee
	}

	feesAt, err := client.ChainHead(ctx)
	req.NoError(err)
	firstAt1x := dailyFee(legacy[0], feesAt.Key())
	secondAt1x := dailyFee(legacy[1], feesAt.Key())
	nativeFee := dailyFee(native[0], feesAt.Key())
	t.Logf("daily fees: 1x %s and %s, native 10x %s", firstAt1x, secondAt1x, nativeFee)
	req.Equal(firstAt1x.String(), secondAt1x.String(), "two sectors of the same tier and batch pay the same")
	req.True(nativeFee.GreaterThan(firstAt1x), "a 10x sector pays more per day than a 1x one")

	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{legacy[0]}, nil)
	req.NoError(err)
	upgradedAt, err := client.ChainHead(ctx)
	req.NoError(err)
	upgradedFee := dailyFee(legacy[0], upgradedAt.Key())
	t.Logf("daily fee after the upgrade: %s, against its untouched sibling's %s", upgradedFee, secondAt1x)
	req.True(upgradedFee.GreaterThan(firstAt1x), "an upgrade must re-derive the fee upward")
	req.True(upgradedFee.GreaterThan(secondAt1x), "the upgraded sector must leave its 1x sibling behind")

	read, err := client.ChainHead(ctx)
	req.NoError(err)
	minerQAP, _ := client.MinerQAP(ctx, maddr, read.Key())
	req.Equal(unit*(10+1+10), minerQAP,
		"miner %s holds one upgraded, one untouched and one native sector", maddr)

	// Partitions carry their members' own tiers, not one multiplier for the whole partition.
	store := client.Store(ctx)
	deadlines, err := client.MinerState(ctx, maddr, read.Key()).LoadDeadlines(store)
	req.NoError(err)

	partitionTotal := big.Zero()
	req.NoError(deadlines.ForEach(store, func(dlIdx uint64, dl *stminer.Deadline) error {
		partitions, err := dl.PartitionsArray(store)
		if err != nil {
			return err
		}
		var part stminer.Partition
		return partitions.ForEach(&part, func(partIdx int64) error {
			partitionTotal = big.Add(partitionTotal, part.ActivePower().QA)

			sectors, err := part.Sectors.All(1 << 20)
			if err != nil {
				return err
			}
			var byTier uint64
			for _, sn := range sectors {
				info := client.MustSectorInfo(ctx, maddr, abi.SectorNumber(sn), read.Key())
				if info.Flags&miner.FULL_QA_POWER != 0 {
					byTier += unit * 10
				} else {
					byTier += unit
				}
			}
			req.Equal(byTier, part.ActivePower().QA.Uint64(),
				"deadline %d partition %d must carry the sum of its members' own tiers", dlIdx, partIdx)
			return nil
		})
	}))
	req.Equal(minerQAP, partitionTotal.Uint64(),
		"miner %s's partitions must add up to its power", maddr)

	um.AssertNoWindowPostError()
}
