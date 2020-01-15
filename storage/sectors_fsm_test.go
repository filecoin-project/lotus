package storage

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/statemachine"
)

func (t *test) planSingle(evt interface{}) {
	_, err := t.m.plan([]statemachine.Event{{evt}}, t.state)
	require.NoError(t.t, err)
}

type test struct {
	m     *Miner
	t     *testing.T
	state *SectorInfo

	next func(statemachine.Context, SectorInfo) error
}

func TestHappyPath(t *testing.T) {
	m := test{
		m:     &Miner{},
		t:     t,
		state: &SectorInfo{State: api.Packing},
	}

	m.planSingle(SectorPacked{})
	require.Equal(m.t, m.state.State, api.Unsealed)

	m.planSingle(SectorSealed{})
	require.Equal(m.t, m.state.State, api.PreCommitting)

	m.planSingle(SectorPreCommitted{})
	require.Equal(m.t, m.state.State, api.PreCommitted)

	m.planSingle(SectorSeedReady{})
	require.Equal(m.t, m.state.State, api.Committing)

	m.planSingle(SectorCommitted{})
	require.Equal(m.t, m.state.State, api.CommitWait)

	m.planSingle(SectorProving{})
	require.Equal(m.t, m.state.State, api.Proving)
}

func TestSeedRevert(t *testing.T) {
	m := test{
		m:     &Miner{},
		t:     t,
		state: &SectorInfo{State: api.Packing},
	}

	m.planSingle(SectorPacked{})
	require.Equal(m.t, m.state.State, api.Unsealed)

	m.planSingle(SectorSealed{})
	require.Equal(m.t, m.state.State, api.PreCommitting)

	m.planSingle(SectorPreCommitted{})
	require.Equal(m.t, m.state.State, api.PreCommitted)

	m.planSingle(SectorSeedReady{})
	require.Equal(m.t, m.state.State, api.Committing)

	_, err := m.m.plan([]statemachine.Event{{SectorSeedReady{}}, {SectorCommitted{}}}, m.state)
	require.NoError(t, err)
	require.Equal(m.t, m.state.State, api.Committing)

	m.planSingle(SectorCommitted{})
	require.Equal(m.t, m.state.State, api.CommitWait)

	m.planSingle(SectorProving{})
	require.Equal(m.t, m.state.State, api.Proving)
}
