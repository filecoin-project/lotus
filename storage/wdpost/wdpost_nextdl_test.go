package wdpost

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	minertypes "github.com/filecoin-project/go-state-types/builtin/v8/miner"
)

func TestNextDeadline(t *testing.T) {
	periodStart := abi.ChainEpoch(0)
	deadlineIdx := 0
	currentEpoch := abi.ChainEpoch(10)

	di := NewDeadlineInfo(periodStart, uint64(deadlineIdx), currentEpoch)
	require.EqualValues(t, 0, di.Index)
	require.EqualValues(t, 0, di.PeriodStart)
	require.EqualValues(t, -20, di.Challenge)
	require.EqualValues(t, 0, di.Open)
	require.EqualValues(t, 60, di.Close)

	for i := 1; i < 1+int(minertypes.WPoStPeriodDeadlines)*2; i++ {
		di = NextDeadline(di)
		deadlineIdx = i % int(minertypes.WPoStPeriodDeadlines)
		expPeriodStart := int(minertypes.WPoStProvingPeriod) * (i / int(minertypes.WPoStPeriodDeadlines))
		expOpen := expPeriodStart + deadlineIdx*int(minertypes.WPoStChallengeWindow)
		expClose := expOpen + int(minertypes.WPoStChallengeWindow)
		expChallenge := expOpen - int(minertypes.WPoStChallengeLookback)
		//fmt.Printf("%d: %d@%d %d-%d (%d)\n", i, expPeriodStart, deadlineIdx, expOpen, expClose, expChallenge)
		require.EqualValues(t, deadlineIdx, di.Index)
		require.EqualValues(t, expPeriodStart, di.PeriodStart)
		require.EqualValues(t, expOpen, di.Open)
		require.EqualValues(t, expClose, di.Close)
		require.EqualValues(t, expChallenge, di.Challenge)
	}
}
