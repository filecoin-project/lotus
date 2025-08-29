//go:build eip7702_enabled

package ethtypes

import (
    "os"
    "testing"

    "github.com/filecoin-project/go-address"
    "github.com/stretchr/testify/require"
)

func TestEnvDelegatorAddr_ConfiguresGlobal(t *testing.T) {
    // Reset and set env
    DelegatorActorAddr = address.Undef
    os.Setenv(EnvDelegatorActorAddr, "ID:1234")
    defer os.Unsetenv(EnvDelegatorActorAddr)

    // Apply env parser explicitly
    applyEnvDelegatorAddr()

    require.NotEqual(t, address.Undef, DelegatorActorAddr)
}
