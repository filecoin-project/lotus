package kit

import (
	"context"
	"testing"
	"time"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	reward19 "github.com/filecoin-project/go-state-types/builtin/v19/reward"
	adt19 "github.com/filecoin-project/go-state-types/builtin/v19/util/adt"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
)

// SolsticeEnv bundles the ensemble pieces (ctx, client, unmanaged miner, miner address, sector
// size, seal proof) produced by NewSolsticeUpgradeEnv for use by the solstice itests.
type SolsticeEnv struct {
	Ctx       context.Context
	Client    *TestFullNode
	Um        *TestUnmanagedMiner
	Maddr     address.Address
	Ssize     abi.SectorSize // always 2KiB
	SealProof abi.RegisteredSealProof
}

// SolsticeOpts configures NewSolsticeUpgradeEnv.
type SolsticeOpts struct {
	// UpgradeEpoch > 0 upgrades the chain NV28 -> NV29 at this height (with the neutral Solstice
	// reward bootstrap so the migration matches the other solstice itests). 0 keeps the chain on
	// NV28 for the whole test.
	UpgradeEpoch abi.ChainEpoch
	// Optional verifreg plumbing: when RootKey/VerifierKey/VerifiedClientKey are non-nil the
	// ensemble is created with a RootVerifier + two funded Accounts (default funding 100 FIL). The
	// same keys are returned to the caller via the enclosing test's own locals, so SetupVerifiedClients
	// / SetupAllocation can reuse them after the ensemble exists.
	RootKey, VerifierKey, VerifiedClientKey *key.Key
	Bal                                     int64
}

// NewSolsticeUpgradeEnv builds the standard solstice itest ensemble: a single unmanaged miner on a
// chain that optionally upgrades NV28->NV29, mock proofs over RPC, mining started, and the miner's
// WindowPoSt driven by its own in-kit loop. It returns the pieces as a *SolsticeEnv for the caller
// to bind.
func NewSolsticeUpgradeEnv(t *testing.T, o SolsticeOpts) *SolsticeEnv {
	t.Helper()
	req := require.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	const ssize = abi.SectorSize(2 << 10) // 2KiB

	sealProof, err := miner.SealProofTypeFromSectorSize(ssize, network.Version28, miner.SealProofVariant_Standard)
	req.NoError(err)

	var ensembleOpts []interface{}
	ensembleOpts = append(ensembleOpts, MockProofs(), ThroughRPC())

	bal := abi.NewTokenAmount(o.Bal)
	if o.Bal == 0 {
		bal = types.FromFil(100)
	}
	if o.RootKey != nil {
		ensembleOpts = append(ensembleOpts,
			RootVerifier(o.RootKey, bal),
			Account(o.VerifierKey, bal),
			Account(o.VerifiedClientKey, bal),
		)
	}

	if o.UpgradeEpoch > 0 {
		ensembleOpts = append(ensembleOpts, UpgradeSchedule(
			stmgr.Upgrade{Network: network.Version28, Height: -1},
			stmgr.Upgrade{
				Network:   network.Version29,
				Height:    o.UpgradeEpoch,
				Migration: filcns.UpgradeActorsV19With(buildconstants.NeutralSolsticeRewardBootstrapParams),
			},
		))
	} else {
		ensembleOpts = append(ensembleOpts, UpgradeSchedule(
			stmgr.Upgrade{Network: network.Version28, Height: -1},
		))
	}

	client, _, ens := EnsembleMinimal(t, ensembleOpts...)

	um, ens := ens.UnmanagedMiner(ctx, client,
		SectorSize(ssize),
		OwnerAddr(client.DefaultKey),
	)
	ctx = ens.UnmanagedContext(ctx)

	ens.InterconnectAll().BeginMining(5 * time.Millisecond)
	ens.Start()

	return &SolsticeEnv{Ctx: ctx, Client: client, Um: um, Maddr: um.ActorAddr, Ssize: ssize, SealProof: sealProof}
}

// WaitForMinerQAP polls the miner's quality-adjusted power (StateMinerPower) until it equals want,
// failing the test after maxWait. Each poll advances the head ~50 epochs.
func WaitForMinerQAP(ctx context.Context, t *testing.T, client *TestFullNode, maddr address.Address, want uint64, maxWait time.Duration) {
	t.Helper()
	endBy := time.Now().Add(maxWait)
	for {
		pw, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
		require.NoError(t, err)
		if pw.MinerPower.QualityAdjPower.Uint64() == want {
			return
		}
		if time.Now().After(endBy) {
			require.FailNowf(t, "QAP wait timeout",
				"miner QAP did not reach %d in time; last=%d", want, pw.MinerPower.QualityAdjPower.Uint64())
		}
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)
		client.WaitTillChain(ctx, HeightAtLeast(head.Height()+50))
	}
}

// LoadReward19 loads the v19 reward actor state and streams, checking invariants.
func LoadReward19(ctx context.Context, t *testing.T, node api.FullNode, store cbor.IpldStore, tsk types.TipSetKey) (*types.Actor, *reward19.State, *reward19.StreamsState) {
	t.Helper()
	req := require.New(t)
	tipset, err := node.ChainGetTipSet(ctx, tsk)
	req.NoError(err)
	parent, err := node.ChainGetTipSet(ctx, tipset.Parents())
	req.NoError(err)
	actor := MustActor(ctx, t, node, builtin.RewardActorAddr, tsk)
	var state reward19.State
	req.NoError(store.Get(ctx, actor.Head, &state))
	adtStore := adt19.WrapStore(ctx, store)
	streams, err := state.LoadStreams(adtStore)
	req.NoError(err)
	_, messages := reward19.CheckStateInvariants(&state, adtStore, parent.Height(), actor.Balance)
	req.Empty(messages.Messages())
	return actor, &state, streams
}
