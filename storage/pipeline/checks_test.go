package sealing

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-statemachine"

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storage/pipeline/mocks"
)

// SubmitCommitAggregate sends a sector without CommR or CommD to CommitFailed, which runs
// checkCommit on it again, so checkCommit must report the missing commitment rather than panic.
func TestCheckCommitMissingCommitments(t *testing.T) {
	sealed, err := cid.Parse("bafkqaaa")
	require.NoError(t, err)
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

			sector := committingSector(tc.commR, tc.commD)
			m, _ := sealingWithPreCommit(t, ctrl, sector, ts, sealed)

			err := m.checkCommit(context.Background(), sector, nil, ts.Key())
			var missing *ErrMissingCommitments
			require.ErrorAs(t, err, &missing)
		})
	}
}

// Returning an error from handleCommitFailed would leave the sector in CommitFailed, so a
// sector without CommR must be sent back to PreCommit1, which recomputes it.
func TestHandleCommitFailedRedoesPreCommitForMissingCommitments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sealed, err := cid.Parse("bafkqaaa")
	require.NoError(t, err)
	ts := makeTestTipSet(t, 300)

	sector := committingSector(nil, &sealed)
	sector.State = CommitFailed
	m, api := sealingWithPreCommit(t, ctrl, sector, ts, sealed)
	api.EXPECT().ChainHead(gomock.Any()).Return(ts, nil)

	evt := runHandlerOnce(t, m.handleCommitFailed, sector)
	require.IsType(t, SectorSealPreCommit1Failed{}, evt)
}

func committingSector(commR, commD *cid.Cid) SectorInfo {
	return SectorInfo{
		SectorNumber: 7,
		SeedEpoch:    200,
		SeedValue:    abi.InteractiveSealRandomness{1, 2, 3},
		CommR:        commR,
		CommD:        commD,
	}
}

// sealingWithPreCommit returns a Sealing whose API serves the sector's precommit and seed, so
// checkCommit gets as far as the sector's CommR and CommD.
func sealingWithPreCommit(t *testing.T, ctrl *gomock.Controller, sector SectorInfo, ts *types.TipSet, sealed cid.Cid) (*Sealing, *mocks.MockSealingAPI) {
	t.Helper()

	maddr, err := address.NewIDAddress(123)
	require.NoError(t, err)
	var entropy bytes.Buffer
	require.NoError(t, maddr.MarshalCBOR(&entropy))

	api := mocks.NewMockSealingAPI(ctrl)
	api.EXPECT().StateSectorPreCommitInfo(gomock.Any(), maddr, sector.SectorNumber, ts.Key()).
		Return(&miner.SectorPreCommitOnChainInfo{
			Info:           miner.SectorPreCommitInfo{SealedCID: sealed},
			PreCommitEpoch: sector.SeedEpoch - policy.GetPreCommitChallengeDelay(),
		}, nil)
	api.EXPECT().StateGetRandomnessFromBeacon(gomock.Any(), crypto.DomainSeparationTag_InteractiveSealChallengeSeed,
		sector.SeedEpoch, entropy.Bytes(), ts.Key()).
		Return(abi.Randomness(sector.SeedValue), nil)

	return &Sealing{Api: api, maddr: maddr}, api
}

type runHandlerEvent struct{}

// handlerRunner plans one run of handler, then reports the first event the handler sends.
type handlerRunner struct {
	handler func(statemachine.Context, SectorInfo) error
	sent    chan interface{}
}

func (r *handlerRunner) Plan(events []statemachine.Event, _ interface{}) (interface{}, uint64, error) {
	if _, ok := events[0].User.(runHandlerEvent); ok {
		return r.handler, 1, nil
	}
	r.sent <- events[0].User
	return nil, 1, nil
}

// runHandlerOnce runs handler on sector with the statemachine.Context of a real state group
// and returns the event the handler sends.
func runHandlerOnce(t *testing.T, handler func(statemachine.Context, SectorInfo) error, sector SectorInfo) interface{} {
	t.Helper()

	runner := &handlerRunner{handler: handler, sent: make(chan interface{}, 1)}
	group := statemachine.New(dssync.MutexWrap(datastore.NewMapDatastore()), runner, SectorInfo{})
	t.Cleanup(func() { _ = group.Stop(context.Background()) })

	id := uint64(sector.SectorNumber)
	require.NoError(t, group.Begin(id, &sector))
	require.NoError(t, group.Send(id, runHandlerEvent{}))

	select {
	case evt := <-runner.sent:
		return evt
	case <-time.After(10 * time.Second):
		t.Fatal("handler did not send an event")
		return nil
	}
}
