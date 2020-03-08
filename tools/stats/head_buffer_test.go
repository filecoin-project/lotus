package main

import (
	"testing"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/stretchr/testify/require"
)

func TestHeadBuffer(t *testing.T) {

	t.Run("Straight push through", func(t *testing.T) {
		hb := NewHeadBuffer(5)
		require.Nil(t, hb.Push(&store.HeadChange{Type: "1"}))
		require.Nil(t, hb.Push(&store.HeadChange{Type: "2"}))
		require.Nil(t, hb.Push(&store.HeadChange{Type: "3"}))
		require.Nil(t, hb.Push(&store.HeadChange{Type: "4"}))
		require.Nil(t, hb.Push(&store.HeadChange{Type: "5"}))

		hc := hb.Push(&store.HeadChange{Type: "6"})
		require.Equal(t, hc.Type, "1")
	})

	t.Run("Reverts", func(t *testing.T) {
		hb := NewHeadBuffer(5)
		require.Nil(t, hb.Push(&store.HeadChange{Type: "1"}))
		require.Nil(t, hb.Push(&store.HeadChange{Type: "2"}))
		require.Nil(t, hb.Push(&store.HeadChange{Type: "3"}))
		hb.Pop()
		require.Nil(t, hb.Push(&store.HeadChange{Type: "3a"}))
		hb.Pop()
		require.Nil(t, hb.Push(&store.HeadChange{Type: "3b"}))
		require.Nil(t, hb.Push(&store.HeadChange{Type: "4"}))
		require.Nil(t, hb.Push(&store.HeadChange{Type: "5"}))

		hc := hb.Push(&store.HeadChange{Type: "6"})
		require.Equal(t, hc.Type, "1")
		hc = hb.Push(&store.HeadChange{Type: "7"})
		require.Equal(t, hc.Type, "2")
		hc = hb.Push(&store.HeadChange{Type: "8"})
		require.Equal(t, hc.Type, "3b")
	})
}
