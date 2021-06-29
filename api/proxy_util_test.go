package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type StrA struct {
	StrB

	Internal struct {
		A int
	}
}

type StrB struct {
	Internal struct {
		B int
	}
}

func TestGetInternalStructs(t *testing.T) {
	var proxy StrA

	sts := GetInternalStructs(&proxy)
	require.Len(t, sts, 2)

	sa := sts[0].(*struct{ A int })
	sa.A = 3
	sb := sts[1].(*struct{ B int })
	sb.B = 4

	require.Equal(t, 3, proxy.Internal.A)
	require.Equal(t, 4, proxy.StrB.Internal.B)
}
