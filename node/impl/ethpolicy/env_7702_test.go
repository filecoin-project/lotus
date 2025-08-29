package ethpolicy

import (
    "os"
    "testing"
    "github.com/stretchr/testify/require"
)

func TestResolveDelegationCap(t *testing.T) {
    // unset
    os.Unsetenv(EnvDelegationCap)
    require.Equal(t, 4, ResolveDelegationCap(4))

    // valid
    os.Setenv(EnvDelegationCap, "7")
    defer os.Unsetenv(EnvDelegationCap)
    require.Equal(t, 7, ResolveDelegationCap(4))

    // invalid
    os.Setenv(EnvDelegationCap, "abc")
    require.Equal(t, 4, ResolveDelegationCap(4))

    os.Setenv(EnvDelegationCap, "-1")
    require.Equal(t, 4, ResolveDelegationCap(4))
}

