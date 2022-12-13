package bcast

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/multiformats/go-multihash"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
)

var log = logging.Logger("sub-cb")

const (
	// GcSanityCheck determines the number of epochs that in the past
	// that will be garbage collected from the current epoch.
	GcSanityCheck = 5
	// GcLookback determines the number of epochs kept in the consistent
	// broadcast cache.
	GcLookback = 2
)

type blksInfo struct {
	ctx    context.Context
	cancel context.CancelFunc
	blks   []cid.Cid
}

type bcastDict struct {
	// thread-safe map impl for the dictionary
	// sync.Map accepts `any` as keys and values.
	// To make it type safe and only support the right
	// types we use this auxiliary type.
	m *sync.Map
}

func (bd *bcastDict) load(key multihash.Multihash) (*blksInfo, bool) {
	v, ok := bd.m.Load(key.String())
	if !ok {
		return nil, ok
	}
	return v.(*blksInfo), ok
}

func (bd *bcastDict) store(key multihash.Multihash, d *blksInfo) {
	bd.m.Store(key.String(), d)
}

type ConsistentBCast struct {
	lk    sync.Mutex
	delay time.Duration
	// FIXME: Make this a slice??? Less storage but needs indexing logic.
	m map[abi.ChainEpoch]*bcastDict
}

func newBcastDict() *bcastDict {
	return &bcastDict{new(sync.Map)}
}

// TODO: the VRFProof may already be small enough so we may not need to use a hash here.
// we can maybe bypass the useless computation.
func BCastKey(bh *types.BlockHeader) (multihash.Multihash, error) {
	k := make([]byte, len(bh.Ticket.VRFProof))
	copy(k, bh.Ticket.VRFProof)
	binary.PutVarint(k, int64(bh.Height))
	return multihash.Sum(k, multihash.SHA2_256, -1)
}

func NewConsistentBCast(delay time.Duration) *ConsistentBCast {
	return &ConsistentBCast{
		delay: delay,
		m:     make(map[abi.ChainEpoch]*bcastDict),
	}
}

func cidExists(cids []cid.Cid, c cid.Cid) bool {
	for _, v := range cids {
		if v == c {
			return true
		}
	}
	return false
}

func (bInfo *blksInfo) eqErr() error {
	bInfo.cancel()
	return fmt.Errorf("different blocks with the same ticket already seen")
}

func (cb *ConsistentBCast) Len() int {
	return len(cb.m)
}

func (cb *ConsistentBCast) RcvBlock(ctx context.Context, blk *types.BlockMsg) {
	cb.lk.Lock()
	bcastDict, ok := cb.m[blk.Header.Height]
	if !ok {
		bcastDict = newBcastDict()
		cb.m[blk.Header.Height] = bcastDict
	}
	cb.lk.Unlock()
	key, err := BCastKey(blk.Header)
	if err != nil {
		log.Errorf("couldn't hash blk info for height %d: %s", blk.Header.Height, err)
		return
	}
	blkCid := blk.Cid()

	bInfo, ok := bcastDict.load(key)
	if ok {
		if len(bInfo.blks) > 1 {
			log.Errorf("equivocation detected for height %d: %s", blk.Header.Height, bInfo.eqErr())
			return
		}

		if !cidExists(bInfo.blks, blkCid) {
			bInfo.blks = append(bInfo.blks, blkCid)
			log.Errorf("equivocation detected for height %d: %s", blk.Header.Height, bInfo.eqErr())
			return
		}
		return
	}

	ctx, cancel := context.WithTimeout(ctx, cb.delay)
	bcastDict.store(key, &blksInfo{ctx, cancel, []cid.Cid{blkCid}})
}

func (cb *ConsistentBCast) WaitForDelivery(bh *types.BlockHeader) error {
	bcastDict := cb.m[bh.Height]
	key, err := BCastKey(bh)
	if err != nil {
		return err
	}
	bInfo, ok := bcastDict.load(key)
	if !ok {
		return fmt.Errorf("something went wrong, unknown block with Epoch + VRFProof (cid=%s) in consistent broadcast storage", key)
	}
	// Wait for the timeout
	<-bInfo.ctx.Done()
	if len(bInfo.blks) > 1 {
		return fmt.Errorf("equivocation detected for epoch %d. Two blocks being broadcast with same VRFProof", bh.Height)
	}
	return nil
}

func (cb *ConsistentBCast) GarbageCollect(currEpoch abi.ChainEpoch) {
	cb.lk.Lock()
	defer cb.lk.Unlock()

	// keep currEpoch-2 and delete a few more in the past
	// as a sanity-check
	// Garbage collection is triggered before block delivery,
	// and we use the sanity-check in case there were a few rounds
	// without delivery, and the garbage collection wasn't triggered
	// for a few epochs.
	for i := 0; i < GcSanityCheck; i++ {
		if currEpoch > GcLookback {
			delete(cb.m, currEpoch-abi.ChainEpoch(GcLookback+i))
		}
	}
}
