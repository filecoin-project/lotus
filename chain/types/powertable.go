package types

import "github.com/filecoin-project/go-address"

type PowerTable struct {
	PowerTable []PowerTableEntry
}

type PowerTableEntry struct {
	Miner address.Address
	Power int64
	// jiejie: 可能这里还得加一个pubkey的说，因为没法直接从miner address获得signer key
}

func (pt *PowerTable) Sort() {
	// TODO(jie)
}

func (pt *PowerTable) ApplyDelta(delta []PowerTableEntryDelta) {
	// TODO(jie)
}

// 还要加其他的函数
