package sealing

import (
	"context"
	"testing"

	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-statemachine"

	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func init() {
	_ = logging.SetLogLevel("*", "INFO")
}

func (t *test) planSingle(evt interface{}) {
	_, _, err := t.s.plan([]statemachine.Event{{User: evt}}, t.state)
	require.NoError(t.t, err)
}

type test struct {
	s     *Sealing
	t     *testing.T
	state *SectorInfo
}

func TestHappyPath(t *testing.T) {
	var notif []struct{ before, after SectorInfo }
	ma, _ := address.NewIDAddress(55151)
	m := test{
		s: &Sealing{
			maddr: ma,
			stats: SectorStats{
				bySector: map[abi.SectorID]SectorState{},
				byState:  map[SectorState]int64{},
			},
			notifee: func(before, after SectorInfo) {
				notif = append(notif, struct{ before, after SectorInfo }{before, after})
			},
		},
		t:     t,
		state: &SectorInfo{State: Packing},
	}

	m.planSingle(SectorPacked{})
	require.Equal(m.t, m.state.State, GetTicket)

	m.planSingle(SectorTicket{})
	require.Equal(m.t, m.state.State, PreCommit1)

	m.planSingle(SectorPreCommit1{})
	require.Equal(m.t, m.state.State, PreCommit2)

	m.planSingle(SectorPreCommit2{})
	require.Equal(m.t, m.state.State, SubmitPreCommitBatch)

	m.planSingle(SectorPreCommitBatchSent{})
	require.Equal(m.t, m.state.State, PreCommitBatchWait)

	m.planSingle(SectorPreCommitLanded{})
	require.Equal(m.t, m.state.State, WaitSeed)

	m.planSingle(SectorSeedReady{})
	require.Equal(m.t, m.state.State, Committing)

	m.planSingle(SectorCommitted{})
	require.Equal(m.t, m.state.State, SubmitCommitAggregate)

	m.planSingle(SectorCommitAggregateSent{})
	require.Equal(m.t, m.state.State, CommitAggregateWait)

	m.planSingle(SectorProving{})
	require.Equal(m.t, m.state.State, FinalizeSector)

	m.planSingle(SectorFinalized{})
	require.Equal(m.t, m.state.State, Proving)

	expected := []SectorState{Packing, GetTicket, PreCommit1, PreCommit2, SubmitPreCommitBatch, PreCommitBatchWait, WaitSeed, Committing, SubmitCommitAggregate, CommitAggregateWait, FinalizeSector, Proving}
	for i, n := range notif {
		if n.before.State != expected[i] {
			t.Fatalf("expected before state: %s, got: %s", expected[i], n.before.State)
		}
		if n.after.State != expected[i+1] {
			t.Fatalf("expected after state: %s, got: %s", expected[i+1], n.after.State)
		}
	}
}

func TestHappyPathFinalizeEarly(t *testing.T) {
	var notif []struct{ before, after SectorInfo }
	ma, _ := address.NewIDAddress(55151)
	m := test{
		s: &Sealing{
			maddr: ma,
			stats: SectorStats{
				bySector: map[abi.SectorID]SectorState{},
				byState:  map[SectorState]int64{},
			},
			notifee: func(before, after SectorInfo) {
				notif = append(notif, struct{ before, after SectorInfo }{before, after})
			},
		},
		t:     t,
		state: &SectorInfo{State: Packing},
	}

	m.planSingle(SectorPacked{})
	require.Equal(m.t, m.state.State, GetTicket)

	m.planSingle(SectorTicket{})
	require.Equal(m.t, m.state.State, PreCommit1)

	m.planSingle(SectorPreCommit1{})
	require.Equal(m.t, m.state.State, PreCommit2)

	m.planSingle(SectorPreCommit2{})
	require.Equal(m.t, m.state.State, SubmitPreCommitBatch)

	m.planSingle(SectorPreCommitBatchSent{})
	require.Equal(m.t, m.state.State, PreCommitBatchWait)

	m.planSingle(SectorPreCommitLanded{})
	require.Equal(m.t, m.state.State, WaitSeed)

	m.planSingle(SectorSeedReady{})
	require.Equal(m.t, m.state.State, Committing)

	m.planSingle(SectorProofReady{})
	require.Equal(m.t, m.state.State, CommitFinalize)

	m.planSingle(SectorFinalized{})
	require.Equal(m.t, m.state.State, SubmitCommitAggregate)

	m.planSingle(SectorCommitAggregateSent{})
	require.Equal(m.t, m.state.State, CommitAggregateWait)

	m.planSingle(SectorProving{})
	require.Equal(m.t, m.state.State, FinalizeSector)

	m.planSingle(SectorFinalized{})
	require.Equal(m.t, m.state.State, Proving)

	expected := []SectorState{Packing, GetTicket, PreCommit1, PreCommit2, SubmitPreCommitBatch, PreCommitBatchWait, WaitSeed, Committing, CommitFinalize, SubmitCommitAggregate, CommitAggregateWait, FinalizeSector, Proving}
	for i, n := range notif {
		if n.before.State != expected[i] {
			t.Fatalf("expected before state: %s, got: %s", expected[i], n.before.State)
		}
		if n.after.State != expected[i+1] {
			t.Fatalf("expected after state: %s, got: %s", expected[i+1], n.after.State)
		}
	}
}

func TestCommitFinalizeFailed(t *testing.T) {
	var notif []struct{ before, after SectorInfo }
	ma, _ := address.NewIDAddress(55151)
	m := test{
		s: &Sealing{
			maddr: ma,
			stats: SectorStats{
				bySector: map[abi.SectorID]SectorState{},
				byState:  map[SectorState]int64{},
			},
			notifee: func(before, after SectorInfo) {
				notif = append(notif, struct{ before, after SectorInfo }{before, after})
			},
		},
		t:     t,
		state: &SectorInfo{State: Committing},
	}

	m.planSingle(SectorProofReady{})
	require.Equal(m.t, m.state.State, CommitFinalize)

	m.planSingle(SectorFinalizeFailed{})
	require.Equal(m.t, m.state.State, CommitFinalizeFailed)

	m.planSingle(SectorRetryFinalize{})
	require.Equal(m.t, m.state.State, CommitFinalize)

	m.planSingle(SectorFinalized{})
	require.Equal(m.t, m.state.State, SubmitCommitAggregate)

	expected := []SectorState{Committing, CommitFinalize, CommitFinalizeFailed, CommitFinalize, SubmitCommitAggregate}
	for i, n := range notif {
		if n.before.State != expected[i] {
			t.Fatalf("expected before state: %s, got: %s", expected[i], n.before.State)
		}
		if n.after.State != expected[i+1] {
			t.Fatalf("expected after state: %s, got: %s", expected[i+1], n.after.State)
		}
	}
}
func TestSeedRevert(t *testing.T) {
	ma, _ := address.NewIDAddress(55151)
	m := test{
		s: &Sealing{
			maddr: ma,
			stats: SectorStats{
				bySector: map[abi.SectorID]SectorState{},
				byState:  map[SectorState]int64{},
			},
		},
		t:     t,
		state: &SectorInfo{State: Packing},
	}

	m.planSingle(SectorPacked{})
	require.Equal(m.t, m.state.State, GetTicket)

	m.planSingle(SectorTicket{})
	require.Equal(m.t, m.state.State, PreCommit1)

	m.planSingle(SectorPreCommit1{})
	require.Equal(m.t, m.state.State, PreCommit2)

	m.planSingle(SectorPreCommit2{})
	require.Equal(m.t, m.state.State, SubmitPreCommitBatch)

	m.planSingle(SectorPreCommitBatchSent{})
	require.Equal(m.t, m.state.State, PreCommitBatchWait)

	m.planSingle(SectorPreCommitLanded{})
	require.Equal(m.t, m.state.State, WaitSeed)

	m.planSingle(SectorSeedReady{})
	require.Equal(m.t, m.state.State, Committing)

	_, _, err := m.s.plan([]statemachine.Event{{User: SectorSeedReady{SeedValue: nil, SeedEpoch: 5}}, {User: SectorCommitted{}}}, m.state)
	require.NoError(t, err)
	require.Equal(m.t, m.state.State, Committing)

	// not changing the seed this time
	_, _, err = m.s.plan([]statemachine.Event{{User: SectorSeedReady{SeedValue: nil, SeedEpoch: 5}}, {User: SectorCommitted{}}}, m.state)
	require.NoError(t, err)
	require.Equal(m.t, m.state.State, SubmitCommitAggregate)

	m.planSingle(SectorCommitAggregateSent{})
	require.Equal(m.t, m.state.State, CommitAggregateWait)

	m.planSingle(SectorProving{})
	require.Equal(m.t, m.state.State, FinalizeSector)

	m.planSingle(SectorFinalized{})
	require.Equal(m.t, m.state.State, Proving)
}

func TestPlanCommittingHandlesSectorCommitFailed(t *testing.T) {
	ma, _ := address.NewIDAddress(55151)
	m := test{
		s: &Sealing{
			maddr: ma,
			stats: SectorStats{
				bySector: map[abi.SectorID]SectorState{},
				byState:  map[SectorState]int64{},
			},
		},
		t:     t,
		state: &SectorInfo{State: Committing},
	}

	events := []statemachine.Event{{User: SectorCommitFailed{}}}

	_, err := planCommitting(events, m.state)
	require.NoError(t, err)

	require.Equal(t, CommitFailed, m.state.State)
}

func TestPlannerList(t *testing.T) {
	for state := range ExistSectorStateList {
		_, ok := fsmPlanners[state]
		require.True(t, ok, "state %s", state)
	}

	for state := range fsmPlanners {
		if state == UndefinedSectorState {
			continue
		}
		_, ok := ExistSectorStateList[state]
		require.True(t, ok, "state %s", state)
	}
}

func TestBrokenState(t *testing.T) {
	var notif []struct{ before, after SectorInfo }
	ma, _ := address.NewIDAddress(55151)
	m := test{
		s: &Sealing{
			maddr: ma,
			stats: SectorStats{
				bySector: map[abi.SectorID]SectorState{},
				byState:  map[SectorState]int64{},
			},
			notifee: func(before, after SectorInfo) {
				notif = append(notif, struct{ before, after SectorInfo }{before, after})
			},
		},
		t:     t,
		state: &SectorInfo{State: "not a state"},
	}

	_, _, err := m.s.plan([]statemachine.Event{{User: SectorPacked{}}}, m.state)
	require.Error(t, err)
	require.Equal(m.t, m.state.State, SectorState("not a state"))

	m.planSingle(SectorRemove{})
	require.Equal(m.t, m.state.State, Removing)

	expected := []SectorState{"not a state", "not a state", Removing}
	for i, n := range notif {
		if n.before.State != expected[i] {
			t.Fatalf("expected before state: %s, got: %s", expected[i], n.before.State)
		}
		if n.after.State != expected[i+1] {
			t.Fatalf("expected after state: %s, got: %s", expected[i+1], n.after.State)
		}
	}
}

func TestBadEvent(t *testing.T) {
	var notif []struct{ before, after SectorInfo }
	ma, _ := address.NewIDAddress(55151)
	m := test{
		s: &Sealing{
			maddr: ma,
			stats: SectorStats{
				bySector: map[abi.SectorID]SectorState{},
				byState:  map[SectorState]int64{},
			},
			notifee: func(before, after SectorInfo) {
				notif = append(notif, struct{ before, after SectorInfo }{before, after})
			},
		},
		t:     t,
		state: &SectorInfo{State: Proving},
	}

	_, processed, err := m.s.Plan([]statemachine.Event{{User: SectorPacked{}}}, m.state)
	require.NoError(t, err)
	require.Equal(t, uint64(1), processed)
	require.Equal(m.t, m.state.State, Proving)

	require.Len(t, m.state.Log, 2)
	require.Contains(t, m.state.Log[1].Message, "received unexpected event")
}

func TestTicketExpired(t *testing.T) {
	var notif []struct{ before, after SectorInfo }
	ma, _ := address.NewIDAddress(55151)
	m := test{
		s: &Sealing{
			maddr: ma,
			stats: SectorStats{
				bySector: map[abi.SectorID]SectorState{},
				byState:  map[SectorState]int64{},
			},
			notifee: func(before, after SectorInfo) {
				notif = append(notif, struct{ before, after SectorInfo }{before, after})
			},
		},
		t:     t,
		state: &SectorInfo{State: Packing},
	}

	m.planSingle(SectorPacked{})
	require.Equal(m.t, m.state.State, GetTicket)

	m.planSingle(SectorTicket{})
	require.Equal(m.t, m.state.State, PreCommit1)

	expired := checkTicketExpired(0, MaxTicketAge+1)
	require.True(t, expired)

	m.planSingle(SectorOldTicket{})
	require.Equal(m.t, m.state.State, GetTicket)

	expected := []SectorState{Packing, GetTicket, PreCommit1, GetTicket}
	for i, n := range notif {
		if n.before.State != expected[i] {
			t.Fatalf("expected before state: %s, got: %s", expected[i], n.before.State)
		}
		if n.after.State != expected[i+1] {
			t.Fatalf("expected after state: %s, got: %s", expected[i+1], n.after.State)
		}
	}
}

func TestCreationTimeCleared(t *testing.T) {
	var notif []struct{ before, after SectorInfo }
	ma, _ := address.NewIDAddress(55151)
	m := test{
		s: &Sealing{
			maddr: ma,
			stats: SectorStats{
				bySector: map[abi.SectorID]SectorState{},
				byState:  map[SectorState]int64{},
			},
			notifee: func(before, after SectorInfo) {
				notif = append(notif, struct{ before, after SectorInfo }{before, after})
			},
		},
		t:     t,
		state: &SectorInfo{State: Available},
	}

	// sector starts with zero CreationTime
	m.planSingle(SectorStartCCUpdate{})
	require.Equal(m.t, m.state.State, SnapDealsWaitDeals)

	require.Equal(t, int64(0), m.state.CreationTime)

	// First AddPiece will set CreationTime
	m.planSingle(SectorAddPiece{})
	require.Equal(m.t, m.state.State, SnapDealsAddPiece)

	require.NotEqual(t, int64(0), m.state.CreationTime)

	m.planSingle(SectorPieceAdded{})
	require.Equal(m.t, m.state.State, SnapDealsWaitDeals)

	// abort should clean up CreationTime
	m.planSingle(SectorAbortUpgrade{})
	require.Equal(m.t, m.state.State, AbortUpgrade)

	require.NotEqual(t, int64(0), m.state.CreationTime)

	m.planSingle(SectorRevertUpgradeToProving{})
	require.Equal(m.t, m.state.State, Proving)

	require.Equal(t, int64(0), m.state.CreationTime)

	m.planSingle(SectorMarkForUpdate{})

	// in case CreationTime was set for whatever reason (lotus bug / manual sector state change)
	// make sure we clean it up when starting upgrade
	m.state.CreationTime = 325
	m.planSingle(SectorStartCCUpdate{})
	require.Equal(m.t, m.state.State, SnapDealsWaitDeals)

	require.Equal(t, int64(0), m.state.CreationTime)

	// "First" AddPiece will set CreationTime
	m.planSingle(SectorAddPiece{})
	require.Equal(m.t, m.state.State, SnapDealsAddPiece)

	require.NotEqual(t, int64(0), m.state.CreationTime)
}

func TestRetrySoftErr(t *testing.T) {
	i := 0

	tf := func() error {
		i++
		switch i {
		case 1:
			return storiface.Err(storiface.ErrTempAllocateSpace, xerrors.New("foo"))
		case 2:
			return nil
		default:
			t.Fatalf("what")
			return xerrors.Errorf("this error didn't ever happen, and will never happen")
		}
	}

	err := retrySoftErr(context.Background(), tf)
	require.NoError(t, err)
	require.Equal(t, 2, i)
}
