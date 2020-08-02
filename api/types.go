package api

import (
	"encoding/json"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"

	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	ma "github.com/multiformats/go-multiaddr"
)

// TODO: check if this exists anywhere else

type MultiaddrSlice []ma.Multiaddr

func (m *MultiaddrSlice) UnmarshalJSON(raw []byte) (err error) {
	var temp []string
	if err := json.Unmarshal(raw, &temp); err != nil {
		return err
	}

	res := make([]ma.Multiaddr, len(temp))
	for i, str := range temp {
		res[i], err = ma.NewMultiaddr(str)
		if err != nil {
			return err
		}
	}
	*m = res
	return nil
}

var _ json.Unmarshaler = new(MultiaddrSlice)

type ObjStat struct {
	Size  uint64
	Links uint64
}

type PubsubScore struct {
	ID    peer.ID
	Score *pubsub.PeerScoreSnapshot
}

type MinerInfo struct {
	Owner                      address.Address // Must be an ID-address.
	Worker                     address.Address // Must be an ID-address.
	NewWorker                  address.Address // Must be an ID-address.
	WorkerChangeEpoch          abi.ChainEpoch
	PeerId                     peer.ID
	Multiaddrs                 []abi.Multiaddrs
	SealProofType              abi.RegisteredSealProof
	SectorSize                 abi.SectorSize
	WindowPoStPartitionSectors uint64
}

func NewApiMinerInfo(info miner.MinerInfo) MinerInfo {
	mi := MinerInfo{
		Owner:                      info.Owner,
		Worker:                     info.Worker,
		NewWorker:                  address.Undef,
		WorkerChangeEpoch:          -1,
		PeerId:                     peer.ID(info.PeerId),
		Multiaddrs:                 info.Multiaddrs,
		SealProofType:              info.SealProofType,
		SectorSize:                 info.SectorSize,
		WindowPoStPartitionSectors: info.WindowPoStPartitionSectors,
	}

	if info.PendingWorkerKey != nil {
		mi.NewWorker = info.PendingWorkerKey.NewWorker
		mi.WorkerChangeEpoch = info.PendingWorkerKey.EffectiveAt
	}

	return mi
}
