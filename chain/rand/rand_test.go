package rand_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/rand"
	"github.com/filecoin-project/lotus/chain/stmgr"
)

func init() {
	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1)
	policy.SetConsensusMinerMinPower(abi.NewStoragePower(2048))
	policy.SetMinVerifiedDealSize(abi.NewStoragePower(256))
}

// in v12 and before, if the tipset corresponding to round X is null, we fetch the latest beacon entry BEFORE X that's in a non-null ts
func TestNullRandomnessV1(t *testing.T) {
	ctx := context.Background()
	cg, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		_, err := cg.NextTipSet()
		if err != nil {
			t.Fatal(err)
		}
	}

	offset := cg.CurTipset.Blocks[0].Header.BeaconEntries[len(cg.CurTipset.Blocks[0].Header.BeaconEntries)-1].Round - uint64(cg.CurTipset.TipSet().Height())
	beforeNullHeight := cg.CurTipset.TipSet().Height()

	ts, err := cg.NextTipSetWithNulls(5)
	if err != nil {
		t.Fatal(err)
	}

	entropy := []byte{0, 2, 3, 4}
	// arbitrarily chosen
	pers := crypto.DomainSeparationTag_WinningPoStChallengeSeed

	randEpoch := ts.TipSet.TipSet().Height() - 2

	rand1, err := cg.StateManager().GetRandomnessFromBeacon(ctx, pers, randEpoch, entropy, ts.TipSet.TipSet().Key())
	if err != nil {
		t.Fatal(err)
	}

	bch := cg.BeaconSchedule().BeaconForEpoch(randEpoch).Entry(ctx, uint64(beforeNullHeight)+offset)

	select {
	case resp := <-bch:
		if resp.Err != nil {
			t.Fatal(resp.Err)
		}

		rand2, err := rand.DrawRandomnessFromBase(resp.Entry.Data, pers, randEpoch, entropy)
		if err != nil {
			t.Fatal(err)
		}

		require.Equal(t, rand1, abi.Randomness(rand2))

	case <-ctx.Done():
		t.Fatal("timed out")
	}
}

// at v13, if the tipset corresponding to round X is null, we fetch the latest beacon in the first non-null ts after X
func TestNullRandomnessV2(t *testing.T) {
	ctx := context.Background()

	sched := stmgr.UpgradeSchedule{
		{
			// prepare for upgrade.
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
		},
	}

	cg, err := gen.NewGeneratorWithUpgradeSchedule(sched)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		_, err := cg.NextTipSet()
		if err != nil {
			t.Fatal(err)
		}

	}

	offset := cg.CurTipset.Blocks[0].Header.BeaconEntries[len(cg.CurTipset.Blocks[0].Header.BeaconEntries)-1].Round - uint64(cg.CurTipset.TipSet().Height())

	ts, err := cg.NextTipSetWithNulls(5)
	if err != nil {
		t.Fatal(err)
	}

	entropy := []byte{0, 2, 3, 4}
	// arbitrarily chosen
	pers := crypto.DomainSeparationTag_WinningPoStChallengeSeed

	randEpoch := ts.TipSet.TipSet().Height() - 2

	rand1, err := cg.StateManager().GetRandomnessFromBeacon(ctx, pers, randEpoch, entropy, ts.TipSet.TipSet().Key())
	if err != nil {
		t.Fatal(err)
	}

	bch := cg.BeaconSchedule().BeaconForEpoch(randEpoch).Entry(ctx, uint64(ts.TipSet.TipSet().Height())+offset)

	select {
	case resp := <-bch:
		if resp.Err != nil {
			t.Fatal(resp.Err)
		}

		// note that the randEpoch passed to DrawRandomnessFromBase is still randEpoch (not the latest ts height)
		rand2, err := rand.DrawRandomnessFromBase(resp.Entry.Data, pers, randEpoch, entropy)
		if err != nil {
			t.Fatal(err)
		}

		require.Equal(t, rand1, abi.Randomness(rand2))

	case <-ctx.Done():
		t.Fatal("timed out")
	}
}

// after v14, if the tipset corresponding to round X is null, we still fetch the randomness for X (from the next non-null tipset)
func TestNullRandomnessV3(t *testing.T) {
	ctx := context.Background()
	sched := stmgr.UpgradeSchedule{
		{
			// prepare for upgrade.
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
	}

	cg, err := gen.NewGeneratorWithUpgradeSchedule(sched)

	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		_, err := cg.NextTipSet()
		if err != nil {
			t.Fatal(err)
		}

	}

	ts, err := cg.NextTipSetWithNulls(5)
	if err != nil {
		t.Fatal(err)
	}

	offset := cg.CurTipset.Blocks[0].Header.BeaconEntries[len(cg.CurTipset.Blocks[0].Header.BeaconEntries)-1].Round - uint64(cg.CurTipset.TipSet().Height())

	entropy := []byte{0, 2, 3, 4}
	// arbitrarily chosen
	pers := crypto.DomainSeparationTag_WinningPoStChallengeSeed

	randEpoch := ts.TipSet.TipSet().Height() - 2

	rand1, err := cg.StateManager().GetRandomnessFromBeacon(ctx, pers, randEpoch, entropy, ts.TipSet.TipSet().Key())
	if err != nil {
		t.Fatal(err)
	}

	bch := cg.BeaconSchedule().BeaconForEpoch(randEpoch).Entry(ctx, uint64(randEpoch)+offset)

	select {
	case resp := <-bch:
		if resp.Err != nil {
			t.Fatal(resp.Err)
		}

		rand2, err := rand.DrawRandomnessFromBase(resp.Entry.Data, pers, randEpoch, entropy)
		if err != nil {
			t.Fatal(err)
		}

		require.Equal(t, rand1, abi.Randomness(rand2))

	case <-ctx.Done():
		t.Fatal("timed out")
	}
}
