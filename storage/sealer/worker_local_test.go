package sealer

import (
	"context"
	"testing"
	"time"

	"github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-statestore"

	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type hangStore struct {
	paths.Store

	challengeReads chan struct{}
	unhang         chan struct{}
}

func (s *hangStore) GenerateSingleVanillaProof(ctx context.Context, minerID abi.ActorID, si storiface.PostSectorChallenge, ppt abi.RegisteredPoStProof) ([]byte, error) {
	select {
	case s.challengeReads <- struct{}{}:
	default:
		panic("this shouldn't happen")
	}
	<-s.unhang
	<-s.challengeReads
	return nil, nil
}

func TestWorkerChallengeThrottle(t *testing.T) {
	ctx := context.Background()

	hs := &hangStore{
		challengeReads: make(chan struct{}, 8),
		unhang:         make(chan struct{}),
	}

	wcfg := WorkerConfig{
		MaxParallelChallengeReads: 8,
	}

	lw := NewLocalWorker(wcfg, hs, nil, nil, nil, statestore.New(datastore.NewMapDatastore()))

	var ch []storiface.PostSectorChallenge
	for i := 0; i < 128; i++ {
		ch = append(ch, storiface.PostSectorChallenge{
			SealProof:    0,
			SectorNumber: abi.SectorNumber(i),
		})
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		close(hs.unhang)
	}()

	_, err := lw.GenerateWindowPoSt(ctx, abi.RegisteredPoStProof_StackedDrgWindow32GiBV1, 0, ch, 0, nil)
	require.NoError(t, err)
}
