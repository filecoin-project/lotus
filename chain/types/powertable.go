package types

import "github.com/filecoin-project/go-address"

type PowerTable struct {
	PowerTable []PowerTableEntry
}

type PowerTableEntry struct {
	Miner address.Address
	Power int64
	// TODO(jie): We also need to add pubkey here.
}

func (pt *PowerTable) Sort() {
	// TODO(jie): Implement
}

func (pt *PowerTable) ApplyDelta(delta []PowerTableEntryDelta) {
	// TODO(jie): Implement
}

// TODO(jie): Implement other methods.
