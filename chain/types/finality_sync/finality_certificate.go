package types

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/chain/types"
)

type FinalityCertificate struct {
	GraniteDecision GraniteDecision
	Voters          bitfield.BitField
	BlsSignature    crypto.Signature
}

type GraniteDecision struct {
	InstanceNumber     int64
	FinalizedTipSetKey types.TipSetKey
	Epoch              int64
	PowerTableDelta    []PowerTableEntryDelta
}

type PowerTableEntryDelta struct {
	MinerAddress address.Address
	PowerDelta   int64
}
