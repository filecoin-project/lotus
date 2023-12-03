package types

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/crypto"
)

type FinalityCertificate struct {
	GraniteDecision GraniteDecision
	Voters          bitfield.BitField
	BlsSignature    crypto.Signature
}

type GraniteDecision struct {
	InstanceNumber     int64
	FinalizedTipSetKey TipSetKey
	Epoch              int64
	PowerTableDelta    []PowerTableEntryDelta
}

type PowerTableEntryDelta struct {
	MinerAddress address.Address
	PowerDelta   int64
}

// TODO(jie): Write Serialize() and Deserialize() methods for FinalityCertificate
//   serialized data should of type: []byte
