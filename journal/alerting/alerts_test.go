package alerting

import (
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/journal/mockjournal"
)

func TestAlerting(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	j := mockjournal.NewMockJournal(mockCtrl)

	a := NewAlertingSystem(j)

	j.EXPECT().RegisterEventType("s1", "b1").Return(journal.EventType{System: "s1", Event: "b1"})
	al1 := a.AddAlertType("s1", "b1")

	j.EXPECT().RegisterEventType("s2", "b2").Return(journal.EventType{System: "s2", Event: "b2"})
	al2 := a.AddAlertType("s2", "b2")

	l := a.GetAlerts()
	require.Len(t, l, 2)
	require.Equal(t, al1, l[0].Type)
	require.Equal(t, al2, l[1].Type)

	for _, alert := range l {
		require.False(t, alert.Active)
		require.Nil(t, alert.LastActive)
		require.Nil(t, alert.LastResolved)
	}

	j.EXPECT().RecordEvent(a.alerts[al1].journalType, gomock.Any())
	a.Raise(al1, "test")

	for _, alert := range l { // check for no magic mutations
		require.False(t, alert.Active)
		require.Nil(t, alert.LastActive)
		require.Nil(t, alert.LastResolved)
	}

	l = a.GetAlerts()
	require.Len(t, l, 2)
	require.Equal(t, al1, l[0].Type)
	require.Equal(t, al2, l[1].Type)

	require.True(t, l[0].Active)
	require.NotNil(t, l[0].LastActive)
	require.Equal(t, "raised", l[0].LastActive.Type)
	require.Equal(t, json.RawMessage(`"test"`), l[0].LastActive.Message)
	require.Nil(t, l[0].LastResolved)

	require.False(t, l[1].Active)
	require.Nil(t, l[1].LastActive)
	require.Nil(t, l[1].LastResolved)
}
