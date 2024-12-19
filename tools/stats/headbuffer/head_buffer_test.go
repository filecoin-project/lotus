package headbuffer

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/api"
)

func TestHeadBuffer(t *testing.T) {
	t.Run("Straight Push through", func(t *testing.T) {
		hb := NewHeadChangeStackBuffer(5)
		require.Nil(t, hb.Push(&api.HeadChange{Type: "1"}))
		require.Nil(t, hb.Push(&api.HeadChange{Type: "2"}))
		require.Nil(t, hb.Push(&api.HeadChange{Type: "3"}))
		require.Nil(t, hb.Push(&api.HeadChange{Type: "4"}))
		require.Nil(t, hb.Push(&api.HeadChange{Type: "5"}))

		hc := hb.Push(&api.HeadChange{Type: "6"})
		require.Equal(t, hc.Type, "1")
	})

	t.Run("Reverts", func(t *testing.T) {
		hb := NewHeadChangeStackBuffer(5)
		require.Nil(t, hb.Push(&api.HeadChange{Type: "1"}))
		require.Nil(t, hb.Push(&api.HeadChange{Type: "2"}))
		require.Nil(t, hb.Push(&api.HeadChange{Type: "3"}))
		hb.Pop()
		require.Nil(t, hb.Push(&api.HeadChange{Type: "3a"}))
		hb.Pop()
		require.Nil(t, hb.Push(&api.HeadChange{Type: "3b"}))
		require.Nil(t, hb.Push(&api.HeadChange{Type: "4"}))
		require.Nil(t, hb.Push(&api.HeadChange{Type: "5"}))

		hc := hb.Push(&api.HeadChange{Type: "6"})
		require.Equal(t, hc.Type, "1")
		hc = hb.Push(&api.HeadChange{Type: "7"})
		require.Equal(t, hc.Type, "2")
		hc = hb.Push(&api.HeadChange{Type: "8"})
		require.Equal(t, hc.Type, "3b")
	})
}
