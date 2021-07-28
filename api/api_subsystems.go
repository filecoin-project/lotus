package api

import (
	"bytes"
	"encoding/json"
)

type MinerSubsystems []MinerSubsystem

func (ms MinerSubsystems) Has(entry MinerSubsystem) bool {
	for _, v := range ms {
		if v == entry {
			return true
		}

	}
	return false
}

type MinerSubsystem int

const (
	MarketsSubsystem MinerSubsystem = iota
	MiningSubsystem
	SealingSubsystem
	SectorStorageSubsystem
)

func (ms MinerSubsystem) String() string {
	return MinerSubsystemToString[ms]
}

var MinerSubsystemToString = map[MinerSubsystem]string{
	MarketsSubsystem:       "Markets",
	MiningSubsystem:        "Mining",
	SealingSubsystem:       "Sealing",
	SectorStorageSubsystem: "SectorStorage",
}

var MinerSubsystemToID = map[string]MinerSubsystem{
	"Markets":       MarketsSubsystem,
	"Mining":        MiningSubsystem,
	"Sealing":       SealingSubsystem,
	"SectorStorage": SectorStorageSubsystem,
}

func (ms MinerSubsystem) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	buffer.WriteString(MinerSubsystemToString[ms])
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

func (ms *MinerSubsystem) UnmarshalJSON(b []byte) error {
	var j string
	err := json.Unmarshal(b, &j)
	if err != nil {
		return err
	}
	// TODO: handle zero value
	*ms = MinerSubsystemToID[j]
	return nil
}
