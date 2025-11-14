package ethtypes

import "github.com/filecoin-project/go-address"

// Stub declarations for builds without the eip7702_enabled tag so references compile.
// EthAccountApplyAndCallActorAddr is the primary 0x04 target on this branch.
// EvmApplyAndCallActorAddr remains as a deprecated alias name for historical compatibility.
var EvmApplyAndCallActorAddr address.Address
var EthAccountApplyAndCallActorAddr address.Address
