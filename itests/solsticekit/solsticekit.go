// Package solsticekit holds shared helpers for the Solstice (nv29) integration tests.
//
// The lotus itest CI compiles and runs each *_test.go in itests as an independent unit via
// `go test ./itests/<file>_test.go`, so helpers may not live in sibling _test.go files. Any code
// shared across the solstice itest files therefore lives here as a normal (non-test) package that
// each test file imports.
package solsticekit

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
)

// Env bundles the ensemble pieces (ctx, client, unmanaged miner, miner address, sector size, seal
// proof) produced by NewUpgradeEnv for use by the solstice itests.
type Env struct {
	Ctx       context.Context
	Client    *kit.TestFullNode
	Um        *kit.TestUnmanagedMiner
	Maddr     address.Address
	Ssize     abi.SectorSize // always 2KiB
	SealProof abi.RegisteredSealProof
}

// Opts configures NewUpgradeEnv.
type Opts struct {
	// UpgradeEpoch > 0 upgrades the chain NV28 -> NV29 at this height (with the neutral Solstice
	// reward bootstrap so the migration matches the other solstice itests). 0 keeps the chain on
	// NV28 for the whole test.
	UpgradeEpoch abi.ChainEpoch
	// WatchPost drives the miner's WindowPoSt on the block miner (the common case). Tests that
	// deliberately do not watch (e.g. a worker handover that strands WindowPoSt) set it false.
	WatchPost bool
	// Optional verifreg plumbing: when RootKey/VerifierKey/VerifiedClientKey are non-nil the
	// ensemble is created with a RootVerifier + two funded Accounts (default funding 100 FIL). The
	// same keys are returned to the caller via the enclosing test's own locals, so SetupVerifiedClients
	// / SetupAllocation can reuse them after the ensemble exists.
	RootKey, VerifierKey, VerifiedClientKey *key.Key
	Bal                                     int64
}

// NewUpgradeEnv builds the standard solstice itest ensemble: a single unmanaged miner on a chain
// that optionally upgrades NV28->NV29, mock proofs over RPC, mining started, and (by default) the
// miner's WindowPoSt watched. It returns the pieces as an *Env for the caller to bind.
func NewUpgradeEnv(t *testing.T, o Opts) *Env {
	t.Helper()
	req := require.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	const ssize = abi.SectorSize(2 << 10) // 2KiB

	sealProof, err := miner.SealProofTypeFromSectorSize(ssize, network.Version28, miner.SealProofVariant_Standard)
	req.NoError(err)

	var ensembleOpts []interface{}
	ensembleOpts = append(ensembleOpts, kit.MockProofs(), kit.ThroughRPC())

	bal := o.Bal
	if bal == 0 {
		bal = types.MustParseFIL("100fil").Int64()
	}
	if o.RootKey != nil {
		ensembleOpts = append(ensembleOpts,
			kit.RootVerifier(o.RootKey, abi.NewTokenAmount(bal)),
			kit.Account(o.VerifierKey, abi.NewTokenAmount(bal)),
			kit.Account(o.VerifiedClientKey, abi.NewTokenAmount(bal)),
		)
	}

	if o.UpgradeEpoch > 0 {
		ensembleOpts = append(ensembleOpts, kit.UpgradeSchedule(
			stmgr.Upgrade{Network: network.Version28, Height: -1},
			stmgr.Upgrade{
				Network:   network.Version29,
				Height:    o.UpgradeEpoch,
				Migration: filcns.UpgradeActorsV19With(buildconstants.NeutralSolsticeRewardBootstrapParams),
			},
		))
	} else {
		ensembleOpts = append(ensembleOpts, kit.UpgradeSchedule(
			stmgr.Upgrade{Network: network.Version28, Height: -1},
		))
	}

	client, _, ens := kit.EnsembleMinimal(t, ensembleOpts...)

	um, ens := ens.UnmanagedMiner(ctx, client,
		kit.SectorSize(ssize),
		kit.OwnerAddr(client.DefaultKey),
	)

	blockMiners := ens.InterconnectAll().BeginMining(5 * time.Millisecond)
	ens.Start()
	if o.WatchPost {
		blockMiners[0].WatchMinerForPost(um.ActorAddr)
	}

	return &Env{Ctx: ctx, Client: client, Um: um, Maddr: um.ActorAddr, Ssize: ssize, SealProof: sealProof}
}

// WaitForMinerQAP polls the miner's quality-adjusted power (StateMinerPower) until it equals want,
// failing the test after maxWait. Each poll advances the head ~50 epochs.
func WaitForMinerQAP(t *testing.T, ctx context.Context, client *kit.TestFullNode, maddr address.Address, want uint64, maxWait time.Duration) {
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
		client.WaitTillChain(ctx, kit.HeightAtLeast(head.Height()+50))
	}
}
