package full_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl/full"
)

func init() {
	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1)
	policy.SetConsensusMinerMinPower(abi.NewStoragePower(2048))
	policy.SetMinVerifiedDealSize(abi.NewStoragePower(256))
}

// similar to chain/rand/rand_test.go
func TestStateGetBeaconEntry(t *testing.T) {
	// Ref: https://github.com/filecoin-project/lotus/issues/12414#issuecomment-2320034935
	type expectedBeaconStrategy int
	const (
		expectedBeaconStrategy_beforeNulls expectedBeaconStrategy = iota
		expectedBeaconStrategy_afterNulls
		expectedBeaconStrategy_exact
	)

	testCases := []struct {
		name          string
		nv            network.Version
		strategy      expectedBeaconStrategy // how to determine which round to expect
		wait          bool                   // whether the test should wait for a future round
		negativeEpoch bool
	}{
		{
			// In v12 and before, if the tipset corresponding to round X is null, we fetch the latest beacon entry BEFORE X that's in a non-null ts
			name:     "pre-nv12@1 nulls",
			nv:       network.Version1,
			strategy: expectedBeaconStrategy_beforeNulls,
		},
		{
			name:     "pre-nv12@9 nulls",
			nv:       network.Version9,
			strategy: expectedBeaconStrategy_beforeNulls,
		},
		{
			name:     "pre-nv12@10 nulls",
			nv:       network.Version10,
			strategy: expectedBeaconStrategy_beforeNulls,
		},
		{
			name:     "pre-nv12@12 nulls",
			nv:       network.Version12,
			strategy: expectedBeaconStrategy_beforeNulls,
		},
		{
			name:     "pre-nv12 wait for future round",
			nv:       network.Version12,
			strategy: expectedBeaconStrategy_exact,
			wait:     true,
		},
		{
			name:          "pre-nv12 requesting negative epoch",
			nv:            network.Version12,
			negativeEpoch: true,
		},
		{
			// At v13, if the tipset corresponding to round X is null, we fetch the latest beacon entry in the first non-null ts after X
			name:     "nv13 nulls",
			nv:       network.Version13,
			strategy: expectedBeaconStrategy_afterNulls,
		},
		{
			name:          "nv13 requesting negative epoch",
			nv:            network.Version13,
			negativeEpoch: true,
		},
		{
			name:     "nv13 wait for future round",
			nv:       network.Version13,
			strategy: expectedBeaconStrategy_exact,
			wait:     true,
		},
		{
			// After v14, if the tipset corresponding to round X is null, we still fetch the randomness for X (from the next non-null tipset) but can get the exact round
			name:     "nv14+ nulls",
			nv:       network.Version14,
			strategy: expectedBeaconStrategy_exact,
		},
		{
			name:     "nv14+ wait for future round",
			nv:       network.Version14,
			strategy: expectedBeaconStrategy_exact,
			wait:     true,
		},
		{
			name:          "nv14 requesting negative epoch",
			nv:            network.Version14,
			negativeEpoch: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := require.New(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Setup the necessary (and usable upgrades) to test what we need
			upgrades := stmgr.UpgradeSchedule{}
			for _, upg := range []stmgr.Upgrade{
				{
					Network:   network.Version9,
					Height:    1,
					Migration: filcns.UpgradeActorsV2,
				}, {
					Network:   network.Version10,
					Height:    2,
					Migration: filcns.UpgradeActorsV3,
				}, {
					Network:   network.Version12,
					Height:    3,
					Migration: filcns.UpgradeActorsV4,
				}, {
					Network:   network.Version13,
					Height:    4,
					Migration: filcns.UpgradeActorsV5,
				}, {
					Network:   network.Version14,
					Height:    5,
					Migration: filcns.UpgradeActorsV6,
				},
			} {
				if upg.Network > tc.nv {
					break
				}
				upgrades = append(upgrades, upg)
			}

			// New chain generator
			cg, err := gen.NewGeneratorWithUpgradeSchedule(upgrades)
			req.NoError(err)

			// Mine enough blocks to get through any upgrades
			for i := 0; i < 10; i++ {
				_, err := cg.NextTipSet()
				req.NoError(err)
			}

			heightBeforeNulls := cg.CurTipset.TipSet().Height()

			// Mine a new block but behave as if there were 5 null blocks before it
			ts, err := cg.NextTipSetWithNulls(5)
			req.NoError(err)

			// Offset of drand epoch to filecoin epoch for easier calculation later
			drandOffset := cg.CurTipset.Blocks[0].Header.BeaconEntries[len(cg.CurTipset.Blocks[0].Header.BeaconEntries)-1].Round - uint64(cg.CurTipset.TipSet().Height())
			// Epoch at which we want to get the beacon entry
			randEpoch := ts.TipSet.TipSet().Height() - 2

			mockBeacon := cg.BeaconSchedule()[0].Beacon.(*beacon.MockBeacon)
			if tc.wait {
				randEpoch = ts.TipSet.TipSet().Height() + 1 // in the future
				// Set the max index to the height of the tipset + the offset to make the calls block, waiting for a future round
				mockBeacon.SetMaxIndex(int(ts.TipSet.TipSet().Height())+int(drandOffset), false)
			}

			state := &full.StateAPI{
				Chain:        cg.ChainStore(),
				StateManager: cg.StateManager(),
				Beacon:       cg.BeaconSchedule(),
			}

			// We will be performing two beacon look-ups in separate goroutines, where tc.wait is true we
			// expect them both to block until we tell the mock beacon to return the beacon entry.
			// Otherwise they should both return immediately.

			var gotBeacon *beacon.Response
			var expectedBeacon *beacon.Response
			gotDoneCh := make(chan struct{})
			expectedDoneCh := make(chan struct{})

			// Get the beacon entry from the state API
			go func() {
				reqEpoch := randEpoch
				if tc.negativeEpoch {
					reqEpoch = abi.ChainEpoch(-1)
				}
				be, err := state.StateGetBeaconEntry(ctx, reqEpoch)
				if err != nil {
					gotBeacon = &beacon.Response{Err: err}
				} else {
					gotBeacon = &beacon.Response{Entry: *be}
				}
				close(gotDoneCh)
			}()

			// Get the beacon entry directly from the beacon.

			// First, determine which round to expect based on the strategy for the given network version
			var beaconRound uint64
			switch tc.strategy {
			case expectedBeaconStrategy_beforeNulls:
				beaconRound = uint64(heightBeforeNulls)
			case expectedBeaconStrategy_afterNulls:
				beaconRound = uint64(ts.TipSet.TipSet().Height())
			case expectedBeaconStrategy_exact:
				beaconRound = uint64(randEpoch)
			}

			if tc.negativeEpoch {
				// A negative epoch should get the genesis beacon, which is hardwired to round 0, all zeros
				// in our test data
				expectedBeacon = &beacon.Response{Entry: types.BeaconEntry{Data: make([]byte, 32), Round: 0}}
				close(expectedDoneCh)
			} else {
				bch := cg.BeaconSchedule().BeaconForEpoch(randEpoch).Entry(ctx, beaconRound+drandOffset)
				go func() {
					select {
					case resp := <-bch:
						expectedBeacon = &resp
					case <-ctx.Done():
						req.Fail("timed out")
					}
					close(expectedDoneCh)
				}()
			}

			if tc.wait {
				// Wait for the beacon entry to be requested by both the StateGetBeaconEntry call and the
				// BeaconForEpoch.Entry call to be blocking
				req.Eventually(func() bool {
					return mockBeacon.WaitingOnEntryCount() == 2
				}, 5*time.Second, 10*time.Millisecond)

				// just to be sure, make sure the calls are still blocking
				select {
				case <-gotDoneCh:
					req.Fail("should not have received beacon entry yet")
				default:
				}
				select {
				case <-expectedDoneCh:
					req.Fail("should not have received beacon entry yet")
				default:
				}

				// Increment the max index to allow the mock beacon to return the beacon entry to both calls
				mockBeacon.SetMaxIndex(int(ts.TipSet.TipSet().Height())+int(drandOffset)+1, true)
			}

			select {
			case <-gotDoneCh:
			case <-ctx.Done():
				req.Fail("timed out")
			}
			req.NoError(gotBeacon.Err)
			select {
			case <-expectedDoneCh:
			case <-ctx.Done():
				req.Fail("timed out")
			}
			req.NoError(expectedBeacon.Err)

			req.Equal(0, mockBeacon.WaitingOnEntryCount()) // both should be unblocked

			// Compare the expected beacon entry with the one we got
			require.Equal(t, gotBeacon.Entry, expectedBeacon.Entry)
		})
	}
}
