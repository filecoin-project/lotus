package types

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

type MiningBaseInfo struct {
	MinerPower        BigInt
	NetworkPower      BigInt
	Sectors           []builtin.ExtendedSectorInfo
	WorkerKey         address.Address
	SectorSize        abi.SectorSize
	PrevBeaconEntry   BeaconEntry
	BeaconEntries     []BeaconEntry
	EligibleForMining bool
}
