//go:build testground
// +build testground

package build

import "github.com/filecoin-project/lotus/chain/actors/policy"

// Actor consts
// TODO: pieceSize unused from actors
var MinDealDuration, MaxDealDuration = policy.DealDurationBounds(0)
