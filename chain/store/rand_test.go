package store_test

import (
	"context"
	"testing"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/gen"
)

func init() {
	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1)
	policy.SetConsensusMinerMinPower(abi.NewStoragePower(2048))
	policy.SetMinVerifiedDealSize(abi.NewStoragePower(256))
}

// in v12 and before, if the tipset corresponding to round X is null, we fetch the latest beacon entry BEFORE X that's in a non-null ts
func TestNullRandomnessV12(t *testing.T) {
	ctx := context.Background()
	cg, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	var beforeNullHeight abi.ChainEpoch
	for i := 0; i < 10; i++ {
		ts, err := cg.NextTipSet()
		if err != nil {
			t.Fatal(err)
		}

		beforeNullHeight = ts.TipSet.TipSet().Height()
	}

	ts, err := cg.NextTipSetWithNulls(5)
	if err != nil {
		t.Fatal(err)
	}

	entropy := []byte{0, 2, 3, 4}
	// arbitrarily chosen
	pers := crypto.DomainSeparationTag_WinningPoStChallengeSeed

	round := ts.TipSet.TipSet().Height() - 2

	rand1, err := cg.ChainStore().GetBeaconRandomnessLookingBack(ctx, ts.TipSet.Cids(), pers, round, entropy)
	if err != nil {
		t.Fatal(err)
	}

	bch := cg.BeaconSchedule().BeaconForEpoch(round).Entry(ctx, uint64(beforeNullHeight))

	select {
	case resp := <-bch:
		if resp.Err != nil {
			t.Fatal(resp.Err)
		}

		rand2, err := store.DrawRandomness(resp.Entry.Data, pers, round, entropy)
		if err != nil {
			t.Fatal(err)
		}

		require.Equal(t, rand1, rand2)

	case <-ctx.Done():
		t.Fatal("timed out")
	}
}

// at v13, if the tipset corresponding to round X is null, we fetch the latest beacon in the first non-null ts after X
func TestNullRandomnessV13(t *testing.T) {
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

	ts, err := cg.NextTipSetWithNulls(5)
	if err != nil {
		t.Fatal(err)
	}

	entropy := []byte{0, 2, 3, 4}
	// arbitrarily chosen
	pers := crypto.DomainSeparationTag_WinningPoStChallengeSeed

	round := ts.TipSet.TipSet().Height() - 2

	// this is the endpoint called in v13
	rand1, err := cg.ChainStore().GetLatestBeaconRandomnessLookingForward(ctx, ts.TipSet.Cids(), pers, round, entropy)
	if err != nil {
		t.Fatal(err)
	}

	bch := cg.BeaconSchedule().BeaconForEpoch(round).Entry(ctx, uint64(ts.TipSet.TipSet().Height()))

	select {
	case resp := <-bch:
		if resp.Err != nil {
			t.Fatal(resp.Err)
		}

		// note that the round passed to DrawRandomness is still round (not the latest ts height)
		rand2, err := store.DrawRandomness(resp.Entry.Data, pers, round, entropy)
		if err != nil {
			t.Fatal(err)
		}

		require.Equal(t, rand1, rand2)

	case <-ctx.Done():
		t.Fatal("timed out")
	}
}

// after v14, if the tipset corresponding to round X is null, we still fetch the randomness for X (from the next non-null tipset)
func TestNullRandomnessV14(t *testing.T) {
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

	ts, err := cg.NextTipSetWithNulls(5)
	if err != nil {
		t.Fatal(err)
	}

	entropy := []byte{0, 2, 3, 4}
	// arbitrarily chosen
	pers := crypto.DomainSeparationTag_WinningPoStChallengeSeed

	round := ts.TipSet.TipSet().Height() - 2

	// this is the endpoint called in v14
	rand1, err := cg.ChainStore().GetBeaconRandomnessLookingForward(ctx, ts.TipSet.Cids(), pers, round, entropy)
	if err != nil {
		t.Fatal(err)
	}

	bch := cg.BeaconSchedule().BeaconForEpoch(round).Entry(ctx, uint64(round))

	select {
	case resp := <-bch:
		if resp.Err != nil {
			t.Fatal(resp.Err)
		}

		rand2, err := store.DrawRandomness(resp.Entry.Data, pers, round, entropy)
		if err != nil {
			t.Fatal(err)
		}

		require.Equal(t, rand1, rand2)

	case <-ctx.Done():
		t.Fatal("timed out")
	}
}
