package blockstore

import (
	"bytes"
	"context"
	"testing"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
)

type interceptedMemStore struct {
	MemStore
	Gets     []cid.Cid
	Puts     []cid.Cid
	PutManys [][]cid.Cid
	Hass     []cid.Cid
	Views    []cid.Cid
	Sizes    []cid.Cid
}

var _ Blockstore = (*interceptedMemStore)(nil)

func (ims *interceptedMemStore) Get(cid cid.Cid) (blocks.Block, error) {
	ims.Gets = append(ims.Gets, cid)
	return ims.MemStore.Get(cid)
}

func (ims *interceptedMemStore) View(cid cid.Cid, callback func([]byte) error) error {
	ims.Views = append(ims.Views, cid)
	return ims.MemStore.View(cid, callback)
}

func (ims *interceptedMemStore) Has(cid cid.Cid) (bool, error) {
	ims.Hass = append(ims.Hass, cid)
	return ims.MemStore.Has(cid)
}

func (ims *interceptedMemStore) Put(blk blocks.Block) error {
	ims.Puts = append(ims.Puts, blk.Cid())
	return ims.MemStore.Put(blk)
}

func (ims *interceptedMemStore) GetSize(cid cid.Cid) (int, error) {
	ims.Sizes = append(ims.Sizes, cid)
	return ims.MemStore.GetSize(cid)
}

func (ims *interceptedMemStore) PutMany(blks []blocks.Block) error {
	cids := make([]cid.Cid, 0, len(blks))
	for _, blk := range blks {
		cids = append(cids, blk.Cid())
	}
	ims.PutManys = append(ims.PutManys, cids)
	return ims.MemStore.PutMany(blks)
}

func TestGetViewHit(t *testing.T) {
	ims := &interceptedMemStore{MemStore: make(MemStore)}
	c, err := WrapFreecacheCache(context.Background(), ims, FreecacheConfig{Name: "test", BlockCapacity: 1000, ExistsCapacity: 1000})
	require.NoError(t, err)

	blk1 := blocks.NewBlock([]byte("abc"))
	err = c.Put(blk1)
	require.NoError(t, err)

	blk2 := blocks.NewBlock([]byte("def"))
	err = c.Put(blk2)
	require.NoError(t, err)

	has, err := c.Has(blk1.Cid())
	require.NoError(t, err)
	require.True(t, has)

	blk3, err := c.Get(blk1.Cid())
	require.NoError(t, err)
	require.Equal(t, blk1.RawData(), blk3.RawData())

	var viewed bool
	err = c.View(blk2.Cid(), func(b []byte) error {
		if bytes.Equal(b, blk2.RawData()) {
			viewed = true
		}
		return nil
	})
	require.NoError(t, err)
	require.True(t, viewed)

	require.Len(t, ims.Views, 0)
	require.Len(t, ims.Hass, 0)
	require.Len(t, ims.Gets, 0)
	require.Len(t, ims.Puts, 2)
}

func TestGetViewMiss(t *testing.T) {
	ims := &interceptedMemStore{MemStore: make(MemStore)}
	c, err := WrapFreecacheCache(context.Background(), ims, FreecacheConfig{Name: "test", BlockCapacity: 1000, ExistsCapacity: 1000})
	require.NoError(t, err)

	blk1 := blocks.NewBlock([]byte("abc"))
	err = ims.MemStore.Put(blk1) // insert into the underlying memstore directly.
	require.NoError(t, err)

	blk2 := blocks.NewBlock([]byte("def"))
	err = ims.MemStore.Put(blk2) // insert into the underlying memstore directly.
	require.NoError(t, err)

	has, err := c.Has(blk1.Cid())
	require.NoError(t, err)
	require.True(t, has)

	blk3, err := c.Get(blk1.Cid())
	require.NoError(t, err)
	require.NotNil(t, blk3)

	var viewed bool
	err = c.View(blk2.Cid(), func(b []byte) error {
		if bytes.Equal(b, blk2.RawData()) {
			viewed = true
		}
		return nil
	})
	require.NoError(t, err)
	require.True(t, viewed)

	require.Len(t, ims.Hass, 1)  // blk1
	require.Len(t, ims.Gets, 1)  // blk1
	require.Len(t, ims.Views, 1) // blk2
	require.Len(t, ims.Puts, 0)
}

// TestPutExisting puts a block that already exists and is cached, and verifies
// that we do not call the underlying store.
func TestPutExisting(t *testing.T) {
	ims := &interceptedMemStore{MemStore: make(MemStore)}
	c, err := WrapFreecacheCache(context.Background(), ims, FreecacheConfig{Name: "test", BlockCapacity: 1000, ExistsCapacity: 1000})
	require.NoError(t, err)

	blk1 := blocks.NewBlock([]byte("abc"))
	err = c.Put(blk1)
	require.NoError(t, err)

	for i := 0; i < 100; i++ {
		err := c.Put(blk1) // will not go to the underlying store.
		require.Nil(t, err)
	}

	require.Len(t, ims.Puts, 1) // only the first one.
}

// TestPutMany tests that the puts to the underlying store are only a subset
// when some of the CIDs are already present.
func TestPutMany(t *testing.T) {
	ims := &interceptedMemStore{MemStore: make(MemStore)}
	c, err := WrapFreecacheCache(context.Background(), ims, FreecacheConfig{Name: "test", BlockCapacity: 1000, ExistsCapacity: 1000})
	require.NoError(t, err)

	blk1 := blocks.NewBlock([]byte("abc")) // present
	err = c.Put(blk1)
	require.NoError(t, err)

	blk2 := blocks.NewBlock([]byte("def"))
	blk3 := blocks.NewBlock([]byte("fgh"))
	err = c.PutMany([]blocks.Block{blk1, blk2, blk3}) // this only leads to two puts.
	require.NoError(t, err)

	require.Len(t, ims.Puts, 1)
	require.Len(t, ims.PutManys, 1)
	require.Len(t, ims.PutManys[0], 2)
}

func TestGetInexistent(t *testing.T) {
	ims := &interceptedMemStore{MemStore: make(MemStore)}
	c, err := WrapFreecacheCache(context.Background(), ims, FreecacheConfig{Name: "test", BlockCapacity: 1000, ExistsCapacity: 1000})
	require.NoError(t, err)

	blk1 := blocks.NewBlock([]byte("abc"))

	blk2, err := c.Get(blk1.Cid()) // will go to the underlying store, and mark this with exists=false in the cache.
	require.Equal(t, ErrNotFound, err)
	require.Nil(t, blk2)

	for i := 0; i < 100; i++ {
		blk2, err := c.Get(blk1.Cid()) // cache will short circuit.
		require.Equal(t, ErrNotFound, err)
		require.Nil(t, blk2)

		err = c.View(blk1.Cid(), func(_ []byte) error { return nil })
		require.Equal(t, ErrNotFound, err)
	}

	require.Len(t, ims.Gets, 1)  // only have one get to the underlying store.
	require.Len(t, ims.Views, 0) // views are short-circuited.
}

func TestDeleteBlock(t *testing.T) {
	ims := &interceptedMemStore{MemStore: make(MemStore)}
	c, err := WrapFreecacheCache(context.Background(), ims, FreecacheConfig{Name: "test", BlockCapacity: 1000, ExistsCapacity: 1000})
	require.NoError(t, err)

	blk1 := blocks.NewBlock([]byte("abc"))
	err = c.Put(blk1)
	require.NoError(t, err)

	blk2, err := c.Get(blk1.Cid()) // won't get counted.
	require.NoError(t, err)
	require.Equal(t, blk2.RawData(), blk1.RawData())

	err = c.DeleteBlock(blk1.Cid())
	require.NoError(t, err)

	for i := 0; i < 100; i++ {
		blk2, err := c.Get(blk1.Cid())
		require.Equal(t, ErrNotFound, err)
		require.Nil(t, blk2)

		err = c.View(blk1.Cid(), func(_ []byte) error { return nil })
		require.Equal(t, ErrNotFound, err)
	}

	require.Len(t, ims.Gets, 0)  // store wasn't queried.
	require.Len(t, ims.Views, 0) // store wasn't queried.

	AllCaches.Dirty(blk1.Cid())

	for i := 0; i < 100; i++ {
		blk2, err := c.Get(blk1.Cid())
		require.Equal(t, ErrNotFound, err)
		require.Nil(t, blk2)

		err = c.View(blk1.Cid(), func(_ []byte) error { return nil })
		require.Equal(t, ErrNotFound, err)
	}

	require.Len(t, ims.Gets, 1) // queried once.
	require.Len(t, ims.Views, 0)
}

func TestGetSize(t *testing.T) {
	ims := &interceptedMemStore{MemStore: make(MemStore)}
	c, err := WrapFreecacheCache(context.Background(), ims, FreecacheConfig{Name: "test", BlockCapacity: 1000, ExistsCapacity: 1000})
	require.NoError(t, err)

	blk1 := blocks.NewBlock([]byte("abc"))
	err = c.Put(blk1)
	require.NoError(t, err)

	for i := 0; i < 100; i++ {
		size, err := c.GetSize(blk1.Cid()) // cache will short circuit.
		require.NoError(t, err)
		require.Equal(t, len(blk1.RawData()), size)
	}

	require.Len(t, ims.Puts, 1)  // only have one put to the underlying store.
	require.Len(t, ims.Sizes, 0) // no getsizes requests to the underlying store.

	// sneak in an entry directly.
	blk2 := blocks.NewBlock([]byte("def"))
	err = ims.MemStore.Put(blk2)
	require.NoError(t, err)

	size, err := c.GetSize(blk2.Cid())
	require.NoError(t, err)
	require.Equal(t, len(blk2.RawData()), size)

	for i := 0; i < 100; i++ {
		has, err := c.Has(blk2.Cid())
		require.NoError(t, err)
		require.True(t, has)
	}

	require.Len(t, ims.Sizes, 1)
	require.Len(t, ims.Hass, 0) // no hasses.
}

func TestGlobalEvict(t *testing.T) {
	ims := &interceptedMemStore{MemStore: make(MemStore)}
	c, err := WrapFreecacheCache(context.Background(), ims, FreecacheConfig{Name: "test", BlockCapacity: 1000, ExistsCapacity: 1000})
	require.NoError(t, err)

	blk1 := blocks.NewBlock([]byte("abc"))
	err = c.Put(blk1)
	require.NoError(t, err)

	blk2 := blocks.NewBlock([]byte("def"))
	err = c.Put(blk2)
	require.NoError(t, err)

	AllCaches.Dirty(blk2.Cid()) // dirty blk2, the Get will fall through.

	blk3, err := c.Get(blk1.Cid())
	require.NoError(t, err)
	require.NotNil(t, blk3)

	blk4, err := c.Get(blk2.Cid()) // this will fall through to the underlying store.
	require.NoError(t, err)
	require.NotNil(t, blk4)

	require.Len(t, ims.Hass, 0)
	require.Len(t, ims.Gets, 0)
	require.Len(t, ims.Views, 0)
	require.Len(t, ims.Puts, 2)
}
