package types

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/ipfs/go-cid"
)

type PreSeal struct {
	CommR     cid.Cid
	CommD     cid.Cid
	SectorID  abi.SectorNumber
	Deal      market.DealProposal
	ProofType abi.RegisteredProof
}
