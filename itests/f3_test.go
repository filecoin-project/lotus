package itests

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3"
	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	lotus_api "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/lf3"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules"
)

const (
	DefaultBootstrapEpoch                   = 20
	DefaultFinality                         = 5
	BaseNetworkName       gpbft.NetworkName = "test"
)

type testEnv struct {
	nodes   []*kit.TestFullNode
	miners  []*kit.TestMiner
	m       *manifest.Manifest
	t       *testing.T
	testCtx context.Context
	debug   bool
}

// TestF3_Enabled tests that F3 is enabled successfully, i.e. all miners:
//   - are able to bootstrap,
//   - make progress, and
//   - renew their participation lease after it expires.
func TestF3_Enabled(t *testing.T) {
	kit.QuietMiningLogs()

	const blocktime = 100 * time.Millisecond
	e := setup(t, blocktime)
	e.waitTillF3Runs(10 * time.Second)
	e.waitTillAllMinersParticipate(10 * time.Second)
	e.waitTillF3Instance(lf3.ParticipationLeaseTerm+1, 40*time.Second)
	e.requireAllMinersParticipate()
}

// TestF3_InactiveModes tests F3 API behaviors under different inactive states:
// 1. Completely disabled mode (F3 functionality turned off)
// 2. Not-yet-ready state (F3 enabled but not yet operational)
func TestF3_InactiveModes(t *testing.T) {
	kit.QuietMiningLogs()

	testCases := []struct {
		mode                 string
		expectedErrors       map[string]any
		expectedValues       map[string]any
		customValidateReturn map[string]func(t *testing.T, ret []reflect.Value)
	}{
		{
			mode: "disabled",
			expectedErrors: map[string]any{
				"F3GetOrRenewParticipationTicket": lotus_api.ErrF3Disabled,
				"F3Participate":                   lotus_api.ErrF3Disabled,
				"F3GetCertificate":                lotus_api.ErrF3Disabled,
				"F3GetLatestCertificate":          lotus_api.ErrF3Disabled,
				"F3GetManifest":                   lotus_api.ErrF3Disabled,
				"F3GetECPowerTable":               lotus_api.ErrF3Disabled,
				"F3GetF3PowerTable":               lotus_api.ErrF3Disabled,
				"F3IsRunning":                     lotus_api.ErrF3Disabled,
			},
			expectedValues: map[string]any{
				"F3GetOrRenewParticipationTicket": (api.F3ParticipationTicket)(nil),
				"F3Participate":                   api.F3ParticipationLease{},
				"F3GetCertificate":                (*certs.FinalityCertificate)(nil),
				"F3GetLatestCertificate":          (*certs.FinalityCertificate)(nil),
				"F3GetManifest":                   (*manifest.Manifest)(nil),
				"F3GetECPowerTable":               (gpbft.PowerEntries)(nil),
				"F3GetF3PowerTable":               (gpbft.PowerEntries)(nil),
				"F3IsRunning":                     false,
			},
		},
		{
			mode: "not running",
			expectedErrors: map[string]any{
				"F3GetOrRenewParticipationTicket": "ticket is not valid", // not sure why "ticket is not valid" is returned
				"F3Participate":                   "ticket is not valid",
				"F3GetCertificate":                f3.ErrF3NotRunning.Error(),
				"F3GetLatestCertificate":          f3.ErrF3NotRunning.Error(),
			},
			expectedValues: map[string]any{
				"F3GetOrRenewParticipationTicket": (api.F3ParticipationTicket)(nil),
				"F3Participate":                   api.F3ParticipationLease{},
				"F3GetCertificate":                (*certs.FinalityCertificate)(nil),
				"F3GetLatestCertificate":          (*certs.FinalityCertificate)(nil),
				"F3IsRunning":                     false,
			},
			customValidateReturn: map[string]func(t *testing.T, ret []reflect.Value){
				"F3GetECPowerTable": func(t *testing.T, ret []reflect.Value) {
					// special case because it simply returns power table from EC which is not F3 dependent
					require.NotNil(t, ret[0].Interface(), "unexpected return value")
				},
				"F3GetF3PowerTable": func(t *testing.T, ret []reflect.Value) {
					// special case because it simply returns power table from EC which is not F3 dependent
					require.NotNil(t, ret[0].Interface(), "unexpected return value")
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			opts := []any{kit.MockProofs()}
			if tc.mode == "disabled" {
				opts = append(opts, kit.F3Disabled())
			}
			blockTime := 100 * time.Millisecond
			if tc.mode == "not running" {
				m := newTestManifest(BaseNetworkName, 1<<32, blockTime)
				cfg := &lf3.Config{
					BaseNetworkName: BaseNetworkName,
					StaticManifest:  m,
				}
				opts = append(opts, kit.F3Config(cfg))
			}

			client, miner, ens := kit.EnsembleMinimal(t, opts...)
			ens.InterconnectAll().BeginMining(blockTime)
			ens.Start()

			head := client.WaitTillChain(ctx, kit.HeightAtLeast(10))

			rctx := reflect.ValueOf(ctx)
			rtsk := reflect.ValueOf(head.Key())
			rminer := reflect.ValueOf(miner.ActorAddr)
			rticket := reflect.ValueOf([]byte("fish"))
			rone := reflect.ValueOf(uint64(1))

			calls := map[string][]reflect.Value{
				"F3GetOrRenewParticipationTicket": {rctx, rminer, rticket, rone},
				"F3Participate":                   {rctx, rticket},
				"F3GetCertificate":                {rctx, rone},
				"F3GetLatestCertificate":          {rctx},
				"F3GetManifest":                   {rctx},
				"F3GetECPowerTable":               {rctx, rtsk},
				"F3GetF3PowerTable":               {rctx, rtsk},
				"F3IsRunning":                     {rctx},
			}

			for fn, args := range calls {
				t.Run(fn, func(t *testing.T) {
					ret := reflect.ValueOf(client).MethodByName(fn).Call(args)

					if expectedValue, hasExpectedValue := tc.expectedValues[fn]; hasExpectedValue {
						require.Equal(t, expectedValue, ret[0].Interface(), "unexpected return value")
					}

					if expectedError, hasExpectedError := tc.expectedErrors[fn]; hasExpectedError {
						require.NotNil(t, ret[1].Interface(), "expected error got nil")
						switch err := expectedError.(type) {
						case error:
							require.ErrorIs(t, ret[1].Interface().(error), err, "unexpected error")
						case string:
							require.ErrorContains(t, ret[1].Interface().(error), err, "unexpected error")
						}
					} else {
						require.Nil(t, ret[1].Interface(), "unexpected error")
					}

					if validate, hasValidate := tc.customValidateReturn[fn]; hasValidate {
						validate(t, ret)
					}
				})
			}
		})
	}
}

// Tests that pause/resume and rebootstrapping F3 works
func TestF3_Bootstrap(t *testing.T) {
	kit.QuietMiningLogs()

	const (
		bootstrapEpoch = 50
		blocktime      = 100 * time.Millisecond
	)

	staticManif := newTestManifest(BaseNetworkName, bootstrapEpoch, blocktime)

	e := setupWithStaticManifest(t, staticManif, true)

	e.waitTillAllMinersParticipate(10 * time.Second)
	e.waitTillF3Instance(2, 20*time.Second)

	e.requireAllMinersParticipate()
}

func TestF3_JsonRPCErrorsPassThrough(t *testing.T) {
	const blocktime = 100 * time.Millisecond
	e := setup(t, blocktime, kit.ThroughRPC())
	n := e.nodes[0].FullNode

	e.waitTillF3Runs(5 * time.Second)

	lease, err := n.F3Participate(e.testCtx, []byte("fish"))
	require.ErrorIs(t, err, lotus_api.ErrF3ParticipationTicketInvalid)
	require.Zero(t, lease)

	addr, err := address.NewIDAddress(1413)
	require.NoError(t, err)

	ticket, err := n.F3GetOrRenewParticipationTicket(e.testCtx, addr, nil, 100)
	require.ErrorIs(t, err, lotus_api.ErrF3ParticipationTooManyInstances)
	require.Zero(t, ticket)
}

func (e *testEnv) waitTillF3Runs(timeout time.Duration) {
	e.waitFor(func(n *kit.TestFullNode) bool {
		r, err := n.F3IsRunning(e.testCtx)
		require.NoError(e.t, err)
		return r
	}, timeout)
}

func (e *testEnv) waitTillF3Instance(i uint64, timeout time.Duration) {
	e.t.Helper()
	e.waitFor(func(n *kit.TestFullNode) bool {
		e.t.Helper()
		c, err := n.F3GetLatestCertificate(e.testCtx)
		if err != nil {
			require.ErrorContains(e.t, err, f3.ErrF3NotRunning.Error())
			return false
		}
		return c != nil && c.GPBFTInstance >= i
	}, timeout)
}

func (e *testEnv) waitTillAllMinersParticipate(timeout time.Duration) {
	e.t.Helper()
	require.Eventually(e.t, e.allMinersParticipate, timeout, 100*time.Millisecond)
}

func (e *testEnv) requireAllMinersParticipate() {
	e.t.Helper()
	require.True(e.t, e.allMinersParticipate())
}

func (e *testEnv) allMinersParticipate() bool {
	e.t.Helper()
	// Check that:
	//  1) all miners are participating,
	//  2) each miner is participating only via one node, and
	//  3) each node has at least one participant.
	minerIDs := make(map[uint64]struct{})
	for _, miner := range e.miners {
		id, err := address.IDFromAddress(miner.ActorAddr)
		require.NoError(e.t, err)
		minerIDs[id] = struct{}{}
	}
	for _, n := range e.nodes {
		participants, err := n.F3ListParticipants(e.testCtx)
		require.NoError(e.t, err)
		var foundAtLeastOneMiner bool
		for _, participant := range participants {
			if _, found := minerIDs[participant.MinerID]; found {
				delete(minerIDs, participant.MinerID)
				foundAtLeastOneMiner = true
			}
		}
		if !foundAtLeastOneMiner {
			return false
		}
	}
	return len(minerIDs) == 0
}

func (e *testEnv) waitFor(f func(n *kit.TestFullNode) bool, timeout time.Duration) {
	e.t.Helper()
	require.Eventually(e.t, func() bool {
		e.t.Helper()
		defer func() {
			if e.debug {
				var wg sync.WaitGroup
				printProgress := func(index int, n *kit.TestFullNode) {
					defer wg.Done()
					if progress, err := n.F3GetProgress(e.testCtx); err != nil {
						e.t.Logf("Node #%d progress: err: %v", index, err)
					} else {
						e.t.Logf("Node #%d progress: %v", index, progress)
					}
				}
				for i, n := range e.nodes {
					wg.Add(1)
					go printProgress(i, n)
				}
				wg.Wait()
			}
		}()
		for _, n := range e.nodes {
			if !f(n) {
				return false
			}
		}
		return true
	}, timeout, 100*time.Millisecond)
}

// Setup creates a new F3-enabled network with two miners and two full-nodes
//
// The first node returned by the function is directly connected to a miner,
// and the second full-node is an observer that is not directly connected to
// a miner. The last return value is the manifest sender for the network.
func setup(t *testing.T, blocktime time.Duration, opts ...kit.NodeOpt) *testEnv {
	return setupWithStaticManifest(t, newTestManifest(BaseNetworkName, DefaultBootstrapEpoch, blocktime), false, opts...)
}

func newTestManifest(networkName gpbft.NetworkName, bootstrapEpoch int64, blocktime time.Duration) *manifest.Manifest {
	return &manifest.Manifest{
		ProtocolVersion:   manifest.VersionCapability,
		BootstrapEpoch:    bootstrapEpoch,
		NetworkName:       networkName,
		InitialPowerTable: cid.Undef,
		CommitteeLookback: manifest.DefaultCommitteeLookback,
		CatchUpAlignment:  blocktime / 2,
		Gpbft: manifest.GpbftConfig{
			// Use smaller time intervals for more responsive test progress/assertion.
			Delta:                      250 * time.Millisecond,
			DeltaBackOffExponent:       1.3,
			QualityDeltaMultiplier:     manifest.DefaultGpbftConfig.QualityDeltaMultiplier,
			MaxLookaheadRounds:         5,
			ChainProposedLength:        manifest.DefaultGpbftConfig.ChainProposedLength,
			RebroadcastBackoffBase:     500 * time.Millisecond,
			RebroadcastBackoffSpread:   0.1,
			RebroadcastBackoffExponent: 1.3,
			RebroadcastBackoffMax:      1 * time.Second,
		},
		EC: manifest.EcConfig{
			Period:                   blocktime,
			Finality:                 DefaultFinality,
			DelayMultiplier:          manifest.DefaultEcConfig.DelayMultiplier,
			BaseDecisionBackoffTable: manifest.DefaultEcConfig.BaseDecisionBackoffTable,
			HeadLookback:             0,
			Finalize:                 true,
		},
		CertificateExchange: manifest.CxConfig{
			ClientRequestTimeout: manifest.DefaultCxConfig.ClientRequestTimeout,
			ServerRequestTimeout: manifest.DefaultCxConfig.ServerRequestTimeout,
			MinimumPollInterval:  blocktime,
			MaximumPollInterval:  4 * blocktime,
		},
		PubSub:                manifest.DefaultPubSubConfig,
		ChainExchange:         manifest.DefaultChainExchangeConfig,
		PartialMessageManager: manifest.DefaultPartialMessageManagerConfig,
	}
}

func setupWithStaticManifest(t *testing.T, manif *manifest.Manifest, testBootstrap bool, extraOpts ...kit.NodeOpt) *testEnv {
	ctx, stopServices := context.WithCancel(context.Background())
	errgrp, ctx := errgroup.WithContext(ctx)

	blocktime := manif.EC.Period

	t.Cleanup(func() {
		stopServices()
		require.NoError(t, errgrp.Wait())
	})

	cfg := &lf3.Config{
		BaseNetworkName: BaseNetworkName,
		StaticManifest:  manif,
	}

	nodeOpts := []kit.NodeOpt{kit.WithAllSubsystems(), kit.F3Config(cfg)}
	nodeOpts = append(nodeOpts, extraOpts...)
	minerOpts := []kit.NodeOpt{kit.WithAllSubsystems(), kit.ConstructorOpts(node.Override(node.F3Participation, modules.F3Participation))}
	minerOpts = append(minerOpts, extraOpts...)

	var (
		n1, n2, n3     kit.TestFullNode
		m1, m2, m3, m4 kit.TestMiner
	)

	ens := kit.NewEnsemble(t, kit.MockProofs()).
		FullNode(&n1, nodeOpts...).
		FullNode(&n2, nodeOpts...).
		FullNode(&n3, nodeOpts...).
		Miner(&m1, &n1, minerOpts...).
		Miner(&m2, &n2, minerOpts...).
		Miner(&m3, &n3, minerOpts...).
		Miner(&m4, &n3, minerOpts...).
		Start()

	ens.InterconnectAll().BeginMining(blocktime)

	{
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		n1.WaitTillChain(ctx, kit.HeightAtLeast(abi.ChainEpoch(5)))
		cancel()
	}

	e := &testEnv{m: manif, t: t, testCtx: ctx}
	// in case we want to use more full-nodes in the future
	e.nodes = []*kit.TestFullNode{&n1, &n2, &n3}
	e.miners = []*kit.TestMiner{&m1, &m2, &m3, &m4}

	return e
}

// TestAPIChainGetFinalizedTipSet tests the behavior of ChainGetFinalizedTipSet
// under various F3 states, including F3 disabled, nominal operation,
// F3 not ready, F3 internal errors, and implausible F3 certificates.
// This is modelled off similar tests in api_v2_test.go for v2/ChainGetTipSet.
func TestAPIChainGetFinalizedTipSet(t *testing.T) {
	const (
		timeout          = 2 * time.Minute
		blockTime        = 10 * time.Millisecond
		f3FinalizedEpoch = 100 + policy.ChainFinality
		targetHeight     = 20 + f3FinalizedEpoch
	)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	kit.QuietMiningLogs()

	mockF3 := kit.NewMockF3Backend()
	subject, _, network := kit.EnsembleMinimal(t, kit.F3Backend(mockF3), kit.MockProofs())
	network.BeginMining(blockTime)
	subject.WaitTillChain(ctx, kit.HeightAtLeast(targetHeight))

	var (
		heaviest = func(t *testing.T) *types.TipSet {
			head, err := subject.ChainHead(ctx)
			require.NoError(t, err)
			return head
		}
		ecFinalized = func(t *testing.T) *types.TipSet {
			head, err := subject.ChainHead(ctx)
			require.NoError(t, err)
			ecFinalized, err := subject.ChainGetTipSetByHeight(ctx, head.Height()-policy.ChainFinality, head.Key())
			require.NoError(t, err)
			return ecFinalized
		}
		tipSetAtHeight = func(height abi.ChainEpoch) func(t *testing.T) *types.TipSet {
			return func(t *testing.T) *types.TipSet {
				ts, err := subject.ChainGetTipSetByHeight(ctx, height, types.EmptyTSK)
				require.NoError(t, err)
				return ts
			}
		}
		f3Finalized     = tipSetAtHeight(f3FinalizedEpoch)
		internalF3Error = errors.New("lost hearing in left eye")
		plausibleCertAt = func(t *testing.T, epoch abi.ChainEpoch) *certs.FinalityCertificate {
			f3FinalisedTipSet := tipSetAtHeight(epoch)(t)
			return &certs.FinalityCertificate{
				ECChain: &gpbft.ECChain{
					TipSets: []*gpbft.TipSet{{
						Epoch: int64(f3FinalisedTipSet.Height()),
						Key:   f3FinalisedTipSet.Key().Bytes(),
					}},
				},
			}
		}
		implausibleCert = &certs.FinalityCertificate{
			ECChain: &gpbft.ECChain{
				TipSets: []*gpbft.TipSet{{
					Epoch: int64(1413),
					Key:   []byte(`🐠`),
				}},
			},
		}
	)
	for _, test := range []struct {
		name       string
		when       func(t *testing.T)
		wantTipSet func(t *testing.T) *types.TipSet
		wantErr    string
	}{
		{
			name: "f3 disabled falls back to ec",
			when: func(t *testing.T) {
				mockF3.Running = false
			},
			wantTipSet: ecFinalized,
		},
		{
			name: "f3 is nominal",
			when: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = true
				mockF3.LatestCertErr = nil
				mockF3.LatestCert = plausibleCertAt(t, f3FinalizedEpoch)
			},
			wantTipSet: f3Finalized,
		},
		{
			name: "f3 is older than ec falls back to ec",
			when: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = true
				mockF3.LatestCertErr = nil
				mockF3.LatestCert = plausibleCertAt(t, targetHeight-policy.ChainFinality-10)
			},
			wantTipSet: ecFinalized,
		},
		{
			name: "f3 not ready falls back to ec",
			when: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = true
				mockF3.LatestCert = nil
				mockF3.LatestCertErr = api.ErrF3NotReady
			},
			wantTipSet: ecFinalized,
		},
		{
			name: "f3 internal error falls back to ec",
			when: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = true
				mockF3.LatestCert = nil
				mockF3.LatestCertErr = internalF3Error
			},
			wantTipSet: ecFinalized,
		},
		{
			name: "f3 is broken falls back to ec",
			when: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = true
				mockF3.LatestCert = implausibleCert
				mockF3.LatestCertErr = nil
			},
			wantTipSet: ecFinalized,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.when != nil {
				test.when(t)
			}
			stableExecute := kit.MakeStableExecute(ctx, t, heaviest)

			var gotTipSet *types.TipSet
			var err error
			stableTipSet := stableExecute(func() {
				gotTipSet, err = subject.ChainGetFinalizedTipSet(ctx)
			})

			if test.wantErr != "" {
				require.Nil(t, gotTipSet)
				if err != nil {
					require.ErrorContains(t, err, test.wantErr)
				}
			} else {
				require.Nil(t, err)
				if test.wantTipSet != nil {
					wantTs := test.wantTipSet(t)
					t.Logf("Query height %d, ChainGetFinalizedTipSet height %d, Expected height: %d", stableTipSet.Height(), gotTipSet.Height(), wantTs.Height())
					require.Equal(t, wantTs, gotTipSet)
				}
			}
		})
	}
}
