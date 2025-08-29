package ethpolicy

import (
    "os"
    "strconv"
)

const EnvDelegationCap = "LOTUS_ETH_7702_DELEGATION_CAP"

// ResolveDelegationCap returns the configured per-EOA delegation cap if the
// environment variable is set and valid; otherwise returns defaultCap.
func ResolveDelegationCap(defaultCap int) int {
    v := os.Getenv(EnvDelegationCap)
    if v == "" {
        return defaultCap
    }
    n, err := strconv.Atoi(v)
    if err != nil || n < 0 {
        return defaultCap
    }
    return n
}

