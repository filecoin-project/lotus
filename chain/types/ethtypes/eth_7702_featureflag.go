package ethtypes

import (
    "github.com/filecoin-project/go-address"
)

// Eip7702FeatureEnabled toggles building Filecoin messages from 7702 txs.
// Default is false; enable via build tag file.
var Eip7702FeatureEnabled = false

// DelegatorActorAddr should be set when feature is enabled to point at the
// deployed Delegator actor that applies EIP-7702 delegations.
var DelegatorActorAddr address.Address

