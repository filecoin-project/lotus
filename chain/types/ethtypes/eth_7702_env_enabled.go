//go:build eip7702_enabled

package ethtypes

import (
    "os"
    "strings"

    "github.com/filecoin-project/go-address"
)

const EnvDelegatorActorAddr = "LOTUS_ETH_7702_DELEGATOR_ADDR"

// When the 7702 feature is enabled, allow configuring the Delegator actor
// address via environment variable. This assists testing and dev workflows
// without hard-coding addresses.
func init() { applyEnvDelegatorAddr() }

func applyEnvDelegatorAddr() {
    if !Eip7702FeatureEnabled { return }
    if DelegatorActorAddr != address.Undef { return }
    if v := os.Getenv(EnvDelegatorActorAddr); v != "" {
        // Support ID:<num> shorthand for convenience
        if strings.HasPrefix(v, "ID:") {
            if id, err := address.NewIDAddress(parseUint(v[3:])); err == nil {
                DelegatorActorAddr = id
                return
            }
        }
        if a, err := address.NewFromString(v); err == nil {
            DelegatorActorAddr = a
        }
    }
}

func parseUint(s string) uint64 {
    var n uint64
    for i := 0; i < len(s); i++ {
        c := s[i]
        if c < '0' || c > '9' { break }
        n = n*10 + uint64(c-'0')
    }
    return n
}
