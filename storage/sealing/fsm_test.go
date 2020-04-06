package sealing

import (
	"testing"

	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-statemachine"
	"github.com/filecoin-project/lotus/api"
)

func init() {
	_ = logging.SetLogLevel("*", "INFO")
}

func (t *test) planSingle(evt interface{}) {
	_, err := t.s.plan([]statemachine.Event{{evt}}, t.state)
	require.NoError(t.t, err)
}

type test struct {
	s     *Sealing
	t     *testing.T
	state *SectorInfo

	next func(statemachine.Context, SectorInfo) error
}

func TestHappyPath(t *testing.T) {
	m := test{
		s:     &Sealing{},
		t:     t,
		state: &SectorInfo{State: Packing},
	}

	m.planSingle(SectorPacked{})
	require.Equal(m.t, m.state.State, api.PreCommit1)

	m.planSingle(SectorPreCommit1{})
	require.Equal(m.t, m.state.State, api.PreCommit2)

	m.planSingle(SectorPreCommit2{})
	require.Equal(m.t, m.state.State, api.PreCommitting)

	m.planSingle(SectorPreCommitted{})
	require.Equal(m.t, m.state.State, WaitSeed)

	m.planSingle(SectorSeedReady{})
	require.Equal(m.t, m.state.State, Committing)

	m.planSingle(SectorCommitted{})
	require.Equal(m.t, m.state.State, CommitWait)

	m.planSingle(SectorProving{})
	require.Equal(m.t, m.state.State, FinalizeSector)

	m.planSingle(SectorFinalized{})
	require.Equal(m.t, m.state.State, Proving)
}

func TestSeedRevert(t *testing.T) {
	m := test{
		s:     &Sealing{},
		t:     t,
		state: &SectorInfo{State: Packing},
	}

	m.planSingle(SectorPacked{})
	require.Equal(m.t, m.state.State, api.PreCommit1)

	m.planSingle(SectorPreCommit1{})
	require.Equal(m.t, m.state.State, api.PreCommit2)

	m.planSingle(SectorPreCommit2{})
	require.Equal(m.t, m.state.State, api.PreCommitting)

	m.planSingle(SectorPreCommitted{})
	require.Equal(m.t, m.state.State, WaitSeed)

	m.planSingle(SectorSeedReady{})
	require.Equal(m.t, m.state.State, Committing)

	_, err := m.s.plan([]statemachine.Event{{SectorSeedReady{Seed: SealSeed{Epoch: 5}}}, {SectorCommitted{}}}, m.state)
	require.NoError(t, err)
	require.Equal(m.t, m.state.State, Committing)

	// not changing the seed this time
	_, err = m.s.plan([]statemachine.Event{{SectorSeedReady{Seed: SealSeed{Epoch: 5}}}, {SectorCommitted{}}}, m.state)
	require.Equal(m.t, m.state.State, CommitWait)

	m.planSingle(SectorProving{})
	require.Equal(m.t, m.state.State, FinalizeSector)

	m.planSingle(SectorFinalized{})
	require.Equal(m.t, m.state.State, Proving)
}

func TestPlanCommittingHandlesSectorCommitFailed(t *testing.T) {
	m := test{
		s:     &Sealing{},
		t:     t,
		state: &SectorInfo{State: Committing},
	}

	events := []statemachine.Event{{SectorCommitFailed{}}}

	require.NoError(t, planCommitting(events, m.state))

	require.Equal(t, api.CommitFailed, m.state.State)
}
