package evmstorage

import (
	"bytes"
	"context"
	"fmt"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipldcbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"

	kamt "github.com/filecoin-project/go-kamt-ipld"
)

// evmKAMTOptions is the parameterization the EVM actor uses for both
// contract and transient storage (builtin-actors
// actors/evm/src/interpreter/system.rs). None of it is recoverable from
// the blocks; a mismatch here misreads the tree.
func evmKAMTOptions() []kamt.Option {
	return []kamt.Option{
		kamt.UseTreeBitWidth(5),
		kamt.UseMaxArrayWidth(1),
		kamt.UseMinDataDepth(0),
		kamt.UseKeyLength(32),
	}
}

// Stats describes the shape of one storage KAMT. Depths are physical
// (nodes traversed) unless named logical (key levels consumed, counting
// levels skipped by extensions); with bitWidth 5 and 256-bit keys the
// logical key space is 52 levels deep.
type Stats struct {
	Entries         int64 `json:"entries"`
	Nodes           int64 `json:"nodes"`
	UniqueBlocks    int64 `json:"uniqueBlocks"`
	TotalBytes      int64 `json:"totalBytes"`
	MinNodeBytes    int64 `json:"minNodeBytes"`
	MaxNodeBytes    int64 `json:"maxNodeBytes"`
	MaxDepth        int   `json:"maxDepth"`
	MaxLogicalDepth int   `json:"maxLogicalDepth"`
	Links           int64 `json:"links"`
	Buckets         int64 `json:"buckets"`
	Extensions      int64 `json:"extensions"`
	// LevelsSkipped is the total tree levels elided by extensions; each
	// would otherwise be a single-pointer intermediate node.
	LevelsSkipped int64 `json:"levelsSkippedByExtensions"`
	// NodesPerDepth[d] is the number of nodes at physical depth d.
	NodesPerDepth []int64 `json:"nodesPerDepth"`
	// EntriesPerLogicalDepth[d] is the number of entries held in buckets
	// of nodes at logical depth d.
	EntriesPerLogicalDepth []int64 `json:"entriesPerLogicalDepth"`
	// PointersPerNode[n] is the number of nodes with n set bits (1..32).
	PointersPerNode []int64 `json:"pointersPerNodeHistogram"`
	// ExtensionBits[b] is the number of extensions of b bits (multiples
	// of the bit width).
	ExtensionBits map[int]int64 `json:"extensionBitsHistogram"`
	// ValueBytes[n] is the number of entries whose value is n bytes in
	// wire form (big-endian, leading zeros stripped; 0..32).
	ValueBytes []int64 `json:"valueZerolessBytesHistogram"`
}

// recordingBlockstore passes reads through while recording each block's
// size and forwarding each distinct block to an optional sink.
type recordingBlockstore struct {
	base  ipldcbor.IpldBlockstore
	sizes map[cid.Cid]int
	sink  func(block.Block) error
}

func (rbs *recordingBlockstore) Get(ctx context.Context, c cid.Cid) (block.Block, error) {
	blk, err := rbs.base.Get(ctx, c)
	if err != nil {
		return nil, err
	}
	if _, seen := rbs.sizes[c]; !seen {
		rbs.sizes[c] = len(blk.RawData())
		if rbs.sink != nil {
			if err := rbs.sink(blk); err != nil {
				return nil, fmt.Errorf("sinking block %s: %w", c, err)
			}
		}
	}
	return blk, nil
}

func (rbs *recordingBlockstore) Put(_ context.Context, blk block.Block) error {
	return fmt.Errorf("read-only store: refusing to put %s", blk.Cid())
}

// WalkStorage enumerates a storage KAMT. Every slot is passed to visit in
// ascending key order with its key and value left-padded to 32 bytes.
// Every distinct block backing the tree is passed to sink, if non-nil, in
// discovery order starting with the root. Load validation is on: an error
// mentioning canonical form means the chain holds a non-canonical block,
// which is a reportable finding, not a tool failure.
func WalkStorage(ctx context.Context, bs ipldcbor.IpldBlockstore, root cid.Cid, visit func(slot, value [32]byte) error, sink func(block.Block) error) (*Stats, error) {
	rbs := &recordingBlockstore{base: bs, sizes: make(map[cid.Cid]int), sink: sink}
	n, err := kamt.LoadNode(ctx, ipldcbor.NewCborStore(rbs), root, evmKAMTOptions()...)
	if err != nil {
		return nil, fmt.Errorf("loading KAMT root %s: %w", root, err)
	}

	stats := &Stats{
		PointersPerNode: make([]int64, 33),
		ExtensionBits:   make(map[int]int64),
		ValueBytes:      make([]int64, 33),
	}

	// First pass: every node, for stats. Children load into pointer caches,
	// so the value pass below re-fetches nothing.
	err = n.ForEachNode(ctx, func(info kamt.NodeInfo) error {
		stats.Nodes++
		nodeCID := info.CID
		if !nodeCID.Defined() {
			nodeCID = root
		}
		size := int64(rbs.sizes[nodeCID])
		if stats.MinNodeBytes == 0 || size < stats.MinNodeBytes {
			stats.MinNodeBytes = size
		}
		if size > stats.MaxNodeBytes {
			stats.MaxNodeBytes = size
		}
		stats.MaxDepth = max(stats.MaxDepth, info.Depth)
		stats.MaxLogicalDepth = max(stats.MaxLogicalDepth, info.LogicalDepth)
		for len(stats.NodesPerDepth) <= info.Depth {
			stats.NodesPerDepth = append(stats.NodesPerDepth, 0)
		}
		stats.NodesPerDepth[info.Depth]++
		stats.PointersPerNode[min(len(info.Node.Pointers), 32)]++

		for _, p := range info.Node.Pointers {
			if lengthBits, _ := p.Extension(); lengthBits > 0 {
				stats.Extensions++
				stats.LevelsSkipped += int64(lengthBits / 5)
				stats.ExtensionBits[lengthBits]++
			}
			if len(p.KVs) == 0 {
				stats.Links++
				continue
			}
			stats.Buckets++
			for _, kv := range p.KVs {
				stats.Entries++
				for len(stats.EntriesPerLogicalDepth) <= info.LogicalDepth {
					stats.EntriesPerLogicalDepth = append(stats.EntriesPerLogicalDepth, 0)
				}
				stats.EntriesPerLogicalDepth[info.LogicalDepth]++
				_, wireLen, err := decodeU256(kv.Value.Raw)
				if err != nil {
					return fmt.Errorf("value at slot %x: %w", kv.Key, err)
				}
				stats.ValueBytes[wireLen]++
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	stats.UniqueBlocks = int64(len(rbs.sizes))
	for _, size := range rbs.sizes {
		stats.TotalBytes += int64(size)
	}

	if visit == nil {
		return stats, nil
	}
	// Second pass: entries in key order, from the cached nodes.
	err = n.ForEach(ctx, func(k []byte, val *cbg.Deferred) error {
		if len(k) != 32 {
			return fmt.Errorf("expected a 32-byte key, got %d bytes", len(k))
		}
		value, _, err := decodeU256(val.Raw)
		if err != nil {
			return fmt.Errorf("value at slot %x: %w", k, err)
		}
		return visit([32]byte(k), value)
	})
	if err != nil {
		return nil, err
	}
	return stats, nil
}

// decodeU256 decodes an EVM storage value: a CBOR byte string of at most
// 32 big-endian bytes with leading zeros stripped. Returns the value
// left-padded to 32 bytes and its wire length.
func decodeU256(raw []byte) ([32]byte, int, error) {
	var out [32]byte
	b, err := cbg.ReadByteArray(bytes.NewReader(raw), 32)
	if err != nil {
		return out, 0, fmt.Errorf("not a U256 byte string: %w", err)
	}
	copy(out[32-len(b):], b)
	return out, len(b), nil
}
