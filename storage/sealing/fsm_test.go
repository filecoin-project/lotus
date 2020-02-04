package sealing

import (
	"testing"

	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/statemachine"
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
		state: &SectorInfo{State: api.Packing},
	}

	m.planSingle(SectorPacked{})
	require.Equal(m.t, m.state.State, api.Unsealed)

	m.planSingle(SectorSealed{})
	require.Equal(m.t, m.state.State, api.PreCommitting)

	m.planSingle(SectorPreCommitted{})
	require.Equal(m.t, m.state.State, api.WaitSeed)

	m.planSingle(SectorSeedReady{})
	require.Equal(m.t, m.state.State, api.Committing)

	m.planSingle(SectorCommitted{})
	require.Equal(m.t, m.state.State, api.CommitWait)

	m.planSingle(SectorProving{})
	require.Equal(m.t, m.state.State, api.FinalizeSector)

	m.planSingle(SectorFinalized{})
	require.Equal(m.t, m.state.State, api.Proving)
}

func TestSeedRevert(t *testing.T) {
	m := test{
		s:     &Sealing{},
		t:     t,
		state: &SectorInfo{State: api.Packing},
	}

	m.planSingle(SectorPacked{})
	require.Equal(m.t, m.state.State, api.Unsealed)

	m.planSingle(SectorSealed{})
	require.Equal(m.t, m.state.State, api.PreCommitting)

	m.planSingle(SectorPreCommitted{})
	require.Equal(m.t, m.state.State, api.WaitSeed)

	m.planSingle(SectorSeedReady{})
	require.Equal(m.t, m.state.State, api.Committing)

	_, err := m.s.plan([]statemachine.Event{{SectorSeedReady{seed: SealSeed{BlockHeight: 5}}}, {SectorCommitted{}}}, m.state)
	require.NoError(t, err)
	require.Equal(m.t, m.state.State, api.Committing)

	// not changing the seed this time
	_, err = m.s.plan([]statemachine.Event{{SectorSeedReady{seed: SealSeed{BlockHeight: 5}}}, {SectorCommitted{}}}, m.state)
	require.Equal(m.t, m.state.State, api.CommitWait)

	m.planSingle(SectorProving{})
	require.Equal(m.t, m.state.State, api.FinalizeSector)

	m.planSingle(SectorFinalized{})
	require.Equal(m.t, m.state.State, api.Proving)
}

func TestPlanCommittingHandlesSectorCommitFailed(t *testing.T) {
	m := test{
		s:     &Sealing{},
		t:     t,
		state: &SectorInfo{State: api.Committing},
	}

	events := []statemachine.Event{{SectorCommitFailed{}}}

	require.NoError(t, planCommitting(events, m.state))

	require.Equal(t, api.SectorStates[api.CommitFailed], api.SectorStates[m.state.State])
}
