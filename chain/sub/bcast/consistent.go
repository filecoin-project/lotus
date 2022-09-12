package bcast

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
)

// TODO: Take const out of here and make them build params.
const (
	DELAY           = 6 * time.Second
	GC_SANITY_CHECK = 5
	GC_LOOKBACK     = 2
)

type blksInfo struct {
	ctx    context.Context
	cancel context.CancelFunc
	blks   []cid.Cid
}

type bcastDict struct {
	// TODO: Consider making this a KeyMutexed map
	lk   sync.RWMutex
	blks map[cid.Cid]*blksInfo // map[epoch + VRFProof]blksInfo
}

type ConsistentBCast struct {
	lk    sync.Mutex
	delay time.Duration
	// FIXME: Make this a slice??? Less storage but needs indexing logic.
	m map[abi.ChainEpoch]*bcastDict
}

func newBcastDict(delay time.Duration) *bcastDict {
	return &bcastDict{
		blks: make(map[cid.Cid]*blksInfo),
	}
}

// TODO: What if the VRFProof is already small?? We don´t need the CID. Useless computation.
func BCastKey(bh *types.BlockHeader) cid.Cid {
	proof := bh.Ticket.VRFProof
	binary.PutVarint(proof, int64(bh.Height))
	return cid.NewCidV0(multihash.Multihash(proof))
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
	return fmt.Errorf("equivocation error detected. Different block with the same ticket already seen")
}

func (cb *ConsistentBCast) RcvBlock(ctx context.Context, blk *types.BlockMsg) error {
	cb.lk.Lock()
	bcastDict, ok := cb.m[blk.Header.Height]
	if !ok {
		bcastDict = newBcastDict(cb.delay)
	}
	cb.lk.Unlock()
	key := BCastKey(blk.Header)
	blkCid := blk.Cid()

	bcastDict.lk.Lock()
	defer bcastDict.lk.Unlock()
	bInfo, ok := bcastDict.blks[key]
	if ok {
		if len(bInfo.blks) > 1 {
			return bInfo.eqErr()
		}

		if !cidExists(bInfo.blks, blkCid) {
			bInfo.blks = append(bInfo.blks, blkCid)
			return bInfo.eqErr()
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, cb.delay)
	bcastDict.blks[key] = &blksInfo{ctx, cancel, []cid.Cid{blkCid}}
	return nil
}

func (cb *ConsistentBCast) WaitForDelivery(bh *types.BlockHeader) error {
	bcastDict := cb.m[bh.Height]
	key := BCastKey(bh)
	bcastDict.lk.RLock()
	defer bcastDict.lk.RUnlock()
	bInfo, ok := bcastDict.blks[key]
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
	for i := 0; i < GC_SANITY_CHECK; i++ {
		delete(cb.m, currEpoch-abi.ChainEpoch(2-i))
	}
}
