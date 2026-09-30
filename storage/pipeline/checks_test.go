package sealing

import (
	"bytes"
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/storage/pipeline/mocks"
)

// SubmitCommitAggregate sends a sector without CommR or CommD to CommitFailed, which runs
// checkCommit on it again, so checkCommit must report the missing commitment rather than panic.
func TestCheckCommitMissingCommitments(t *testing.T) {
	maddr, err := address.NewIDAddress(123)
	require.NoError(t, err)
	var entropy bytes.Buffer
	require.NoError(t, maddr.MarshalCBOR(&entropy))

	sealed, err := cid.Parse("bafkqaaa")
	require.NoError(t, err)

	const sectorNumber = abi.SectorNumber(7)
	const seedEpoch = abi.ChainEpoch(200)
	seed := abi.InteractiveSealRandomness{1, 2, 3}
	ts := makeTestTipSet(t, 300)

	for _, tc := range []struct {
		name         string
		commR, commD *cid.Cid
	}{
		{name: "nil CommR", commD: &sealed},
		{name: "nil CommD", commR: &sealed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			api := mocks.NewMockSealingAPI(ctrl)

			api.EXPECT().StateSectorPreCommitInfo(gomock.Any(), maddr, sectorNumber, ts.Key()).
				Return(&miner.SectorPreCommitOnChainInfo{
					Info:           miner.SectorPreCommitInfo{SealedCID: sealed},
					PreCommitEpoch: seedEpoch - policy.GetPreCommitChallengeDelay(),
				}, nil)
			api.EXPECT().StateGetRandomnessFromBeacon(gomock.Any(), crypto.DomainSeparationTag_InteractiveSealChallengeSeed,
				seedEpoch, entropy.Bytes(), ts.Key()).
				Return(abi.Randomness(seed), nil)

			m := &Sealing{Api: api, maddr: maddr}
			err := m.checkCommit(context.Background(), SectorInfo{
				SectorNumber: sectorNumber,
				SeedEpoch:    seedEpoch,
				SeedValue:    seed,
				CommR:        tc.commR,
				CommD:        tc.commD,
			}, nil, ts.Key())
			require.ErrorContains(t, err, "nil commR or commD")
		})
	}
}
