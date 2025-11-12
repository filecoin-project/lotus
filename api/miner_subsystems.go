package api

import (
	"encoding/json"
	"slices"
)

// MinerSubsystem represents a miner subsystem. Int and string values are not
// guaranteed to be stable over time.
type MinerSubsystem int

const (
	// SubsystemUnknown is a placeholder for the zero value. It should never
	// be used.
	SubsystemUnknown MinerSubsystem = iota
	// SubsystemMining signifies the mining subsystem.
	SubsystemMining
	// SubsystemSealing signifies the sealing subsystem.
	SubsystemSealing
	// SubsystemSectorStorage signifies the sector storage subsystem.
	SubsystemSectorStorage
)

var MinerSubsystemToString = map[MinerSubsystem]string{
	SubsystemUnknown:       "Unknown",
	SubsystemMining:        "Mining",
	SubsystemSealing:       "Sealing",
	SubsystemSectorStorage: "SectorStorage",
}

var MinerSubsystemToID = map[string]MinerSubsystem{
	"Unknown":       SubsystemUnknown,
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
	return slices.Contains(ms, entry)
}

func (ms MinerSubsystem) String() string {
	s, ok := MinerSubsystemToString[ms]
	if !ok {
		return MinerSubsystemToString[SubsystemUnknown]
	}
	return s
}
