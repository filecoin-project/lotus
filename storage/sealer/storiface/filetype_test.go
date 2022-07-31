package storiface

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileTypeAllow(t *testing.T) {
	// no filters = allow all
	require.True(t, FTCache.Allowed(nil, nil))

	// allow allows matching type
	require.True(t, FTCache.Allowed((FTCache).Strings(), nil))

	// deny denies matching type
	require.False(t, FTCache.Allowed(nil, (FTCache).Strings()))

	// deny has precedence over allow
	require.False(t, FTCache.Allowed((FTCache).Strings(), (FTCache).Strings()))

	// deny allows non-matching types
	require.True(t, FTUnsealed.Allowed(nil, (FTCache).Strings()))

	// allow denies non-matching types
	require.False(t, FTUnsealed.Allowed((FTCache).Strings(), nil))
}

func TestFileTypeAnyAllow(t *testing.T) {
	// no filters = allow all
	require.True(t, FTCache.AnyAllowed(nil, nil))

	// one denied
	require.False(t, FTCache.AnyAllowed(nil, (FTCache).Strings()))
	require.True(t, FTCache.AnyAllowed(nil, (FTUnsealed).Strings()))

	// one denied, one allowed = allowed
	require.True(t, (FTCache|FTUpdateCache).AnyAllowed(nil, (FTCache).Strings()))
}
