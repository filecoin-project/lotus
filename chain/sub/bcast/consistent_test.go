package bcast_test

import (
	"context"
	"crypto/rand"
	"fmt"
	mrand "math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/sub/bcast"
	"github.com/filecoin-project/lotus/chain/types"
)

const TEST_DELAY = 1 * time.Second

func TestSimpleDelivery(t *testing.T) {
	cb := bcast.NewConsistentBCast(TEST_DELAY)
	// Check that we wait for delivery.
	start := time.Now()
	testSimpleDelivery(t, cb, 100, 5)
	since := time.Since(start)
	require.GreaterOrEqual(t, since, TEST_DELAY)
}

func testSimpleDelivery(t *testing.T, cb *bcast.ConsistentBCast, epoch abi.ChainEpoch, numBlocks int) {
	ctx := context.Background()

	wg := new(sync.WaitGroup)
	errs := make([]error, 0)
	wg.Add(numBlocks)
	for i := 0; i < numBlocks; i++ {
		go func(i int) {
			defer wg.Done()
			// Add a random delay in block reception
			r := mrand.Intn(200)
			time.Sleep(time.Duration(r) * time.Millisecond)
			blk := newBlock(t, epoch, randomProof(t), []byte("test"+strconv.Itoa(i)))
			cb.RcvBlock(ctx, blk)
			err := cb.WaitForDelivery(blk.Header)
			if err != nil {
				errs = append(errs, err)
			}
		}(i)
	}
	wg.Wait()

	for _, v := range errs {
		t.Fatalf("error in delivery: %s", v)
	}
}

func TestSeveralEpochs(t *testing.T) {
	cb := bcast.NewConsistentBCast(TEST_DELAY)
	numEpochs := 6
	wg := new(sync.WaitGroup)
	wg.Add(numEpochs)
	for i := 0; i < numEpochs; i++ {
		go func(i int) {
			defer wg.Done()
			// Add a random delay between epochs
			r := mrand.Intn(500)
			time.Sleep(time.Duration(i)*TEST_DELAY + time.Duration(r)*time.Millisecond)
			rNumBlocks := mrand.Intn(5)
			flip, err := flipCoin(0.7)
			require.NoError(t, err)
			t.Logf("Running epoch %d with %d with equivocation=%v", i, rNumBlocks, !flip)
			if flip {
				testSimpleDelivery(t, cb, abi.ChainEpoch(i), rNumBlocks)
			} else {
				testEquivocation(t, cb, abi.ChainEpoch(i), rNumBlocks)
			}
			cb.GarbageCollect(abi.ChainEpoch(i))
		}(i)
	}
	wg.Wait()
	require.Equal(t, cb.Len(), numEpochs)
}

// bias is expected to be 0-1
func flipCoin(bias float32) (bool, error) {
	if bias > 1 || bias < 0 {
		return false, fmt.Errorf("wrong bias. expected (0,1)")
	}
	r := mrand.Intn(100)
	return r < int(bias*100), nil
}

func testEquivocation(t *testing.T, cb *bcast.ConsistentBCast, epoch abi.ChainEpoch, numBlocks int) {
	ctx := context.Background()

	wg := new(sync.WaitGroup)
	errs := make([]error, 0)
	wg.Add(numBlocks + 1)
	for i := 0; i < numBlocks; i++ {
		proof := randomProof(t)
		// Valid blocks
		go func(i int, proof []byte) {
			defer wg.Done()
			r := mrand.Intn(200)
			time.Sleep(time.Duration(r) * time.Millisecond)
			blk := newBlock(t, epoch, proof, []byte("valid"+strconv.Itoa(i)))
			cb.RcvBlock(ctx, blk)
			err := cb.WaitForDelivery(blk.Header)
			if err != nil {
				errs = append(errs, err)
			}
		}(i, proof)

		// Equivocation for the last block
		if i == numBlocks-1 {
			// Attempting equivocation
			go func(i int, proof []byte) {
				defer wg.Done()
				// Use the same proof and the same epoch
				blk := newBlock(t, epoch, proof, []byte("invalid"+strconv.Itoa(i)))
				cb.RcvBlock(ctx, blk)
				err := cb.WaitForDelivery(blk.Header)
				// Equivocation detected
				require.Error(t, err)
			}(i, proof)
		}
	}
	wg.Wait()

	// The equivocated block arrived too late, so
	// we delivered all the valid blocks.
	require.Len(t, errs, 1)
}

func TestEquivocation(t *testing.T) {
	cb := bcast.NewConsistentBCast(TEST_DELAY)
	testEquivocation(t, cb, 100, 5)
}

func TestFailedEquivocation(t *testing.T) {
	cb := bcast.NewConsistentBCast(TEST_DELAY)
	ctx := context.Background()
	numBlocks := 5

	wg := new(sync.WaitGroup)
	errs := make([]error, 0)
	wg.Add(numBlocks + 1)
	for i := 0; i < numBlocks; i++ {
		proof := randomProof(t)
		// Valid blocks
		go func(i int, proof []byte) {
			defer wg.Done()
			r := mrand.Intn(200)
			time.Sleep(time.Duration(r) * time.Millisecond)
			blk := newBlock(t, 100, proof, []byte("valid"+strconv.Itoa(i)))
			cb.RcvBlock(ctx, blk)
			err := cb.WaitForDelivery(blk.Header)
			if err != nil {
				errs = append(errs, err)
			}
		}(i, proof)

		// Equivocation for the last block
		if i == numBlocks-1 {
			// Attempting equivocation
			go func(i int, proof []byte) {
				defer wg.Done()
				// The equivocated block arrives late
				time.Sleep(2 * TEST_DELAY)
				// Use the same proof and the same epoch
				blk := newBlock(t, 100, proof, []byte("invalid"+strconv.Itoa(i)))
				cb.RcvBlock(ctx, blk)
				err := cb.WaitForDelivery(blk.Header)
				// Equivocation detected
				require.Error(t, err)
			}(i, proof)
		}
	}
	wg.Wait()

	// The equivocated block arrived too late, so
	// we delivered all the valid blocks.
	require.Len(t, errs, 0)
}

func randomProof(t *testing.T) []byte {
	proof := make([]byte, 10)
	_, err := rand.Read(proof)
	if err != nil {
		t.Fatal(err)
	}
	return proof
}

func newBlock(t *testing.T, epoch abi.ChainEpoch, proof []byte, mCidSeed []byte) *types.BlockMsg {
	h, err := multihash.Sum(mCidSeed, multihash.SHA2_256, -1)
	if err != nil {
		t.Fatal(err)
	}
	testCid := cid.NewCidV0(h)
	addr, err := address.NewIDAddress(10)
	if err != nil {
		t.Fatal(err)
	}
	bh := &types.BlockHeader{
		Miner:                 addr,
		ParentStateRoot:       testCid,
		ParentMessageReceipts: testCid,
		Ticket: &types.Ticket{
			VRFProof: proof,
		},
		Height:   epoch,
		Messages: testCid,
	}
	return &types.BlockMsg{
		Header: bh,
	}
}
