package api

import (
	"encoding/json"
)

// MinerSubsystem represents a miner subsystem. Int and string values are not
// guaranteed to be stable over time  is not
// guaranteed to be stable over time.
type MinerSubsystem int

const (
	// SubsystemUnknown is a placeholder for the zero value. It should never
	// be used.
	SubsystemUnknown MinerSubsystem = iota
	// SubsystemMarkets signifies the storage and retrieval
	// deal-making subsystem.
	SubsystemMarkets
	// SubsystemMining signifies the mining subsystem.
	SubsystemMining
	// SubsystemSealing signifies the sealing subsystem.
	SubsystemSealing
	// SubsystemSectorStorage signifies the sector storage subsystem.
	SubsystemSectorStorage
)

var MinerSubsystemToString = map[MinerSubsystem]string{
	SubsystemUnknown:       "Unknown",
	SubsystemMarkets:       "Markets",
	SubsystemMining:        "Mining",
	SubsystemSealing:       "Sealing",
	SubsystemSectorStorage: "SectorStorage",
}

var MinerSubsystemToID = map[string]MinerSubsystem{
	"Unknown":       SubsystemUnknown,
	"Markets":       SubsystemMarkets,
	"Mining":        SubsystemMining,
	"Sealing":       SubsystemSealing,
	"SectorStorage": SubsystemSectorStorage,
}

func (ms MinerSubsystem) MarshalJSON() ([]byte, error) {
	return json.Marshal(MinerSubsystemToString[ms])
}

func (ms *MinerSubsystem) UnmarshalJSON(b []byte) error {
	var j string
	err := json.Unmarshal(b, &j)
	if err != nil {
		return err
	}
	s, ok := MinerSubsystemToID[j]
	if !ok {
		*ms = SubsystemUnknown
	} else {
		*ms = s
	}
	return nil
}

type MinerSubsystems []MinerSubsystem

func (ms MinerSubsystems) Has(entry MinerSubsystem) bool {
	for _, v := range ms {
		if v == entry {
			return true
		}
	}
	return false
}

func (ms MinerSubsystem) String() string {
	s, ok := MinerSubsystemToString[ms]
	if !ok {
		return MinerSubsystemToString[SubsystemUnknown]
	}
	return s
}
