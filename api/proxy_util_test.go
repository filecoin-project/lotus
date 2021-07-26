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

type StrC struct {
	Internal struct {
		Internal struct {
			C int
		}
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

func TestNestedInternalStructs(t *testing.T) {
	var proxy StrC

	// check that only the top-level internal struct gets picked up

	sts := GetInternalStructs(&proxy)
	require.Len(t, sts, 1)

	sa := sts[0].(*struct {
		Internal struct {
			C int
		}
	})
	sa.Internal.C = 5

	require.Equal(t, 5, proxy.Internal.Internal.C)
}
