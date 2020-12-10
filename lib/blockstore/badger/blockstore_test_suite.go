package badgerbs

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/filecoin-project/lotus/lib/blockstore"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	u "github.com/ipfs/go-ipfs-util"

	"github.com/stretchr/testify/require"
)

// TODO: move this to go-ipfs-blockstore.
type Suite struct {
	NewBlockstore  func(tb testing.TB) (bs blockstore.Blockstore, path string)
	OpenBlockstore func(tb testing.TB, path string) (bs blockstore.Blockstore, err error)
}

func (s *Suite) RunTests(t *testing.T, prefix string) {
	v := reflect.TypeOf(s)
	f := func(t *testing.T) {
		for i := 0; i < v.NumMethod(); i++ {
			if m := v.Method(i); strings.HasPrefix(m.Name, "Test") {
				f := m.Func.Interface().(func(*Suite, *testing.T))
				t.Run(m.Name, func(t *testing.T) {
					f(s, t)
				})
			}
		}
	}

	if prefix == "" {
		f(t)
	} else {
		t.Run(prefix, f)
	}
}

func (s *Suite) TestGetWhenKeyNotPresent(t *testing.T) {
	bs, _ := s.NewBlockstore(t)
	if c, ok := bs.(io.Closer); ok {
		defer func() { require.NoError(t, c.Close()) }()
	}

	c := cid.NewCidV0(u.Hash([]byte("stuff")))
	bl, err := bs.Get(c)
	require.Nil(t, bl)
	require.Equal(t, blockstore.ErrNotFound, err)
}

func (s *Suite) TestGetWhenKeyIsNil(t *testing.T) {
	bs, _ := s.NewBlockstore(t)
	if c, ok := bs.(io.Closer); ok {
		defer func() { require.NoError(t, c.Close()) }()
	}

	_, err := bs.Get(cid.Undef)
	require.Equal(t, blockstore.ErrNotFound, err)
}

func (s *Suite) TestPutThenGetBlock(t *testing.T) {
	bs, _ := s.NewBlockstore(t)
	if c, ok := bs.(io.Closer); ok {
		defer func() { require.NoError(t, c.Close()) }()
	}

	orig := blocks.NewBlock([]byte("some data"))

	err := bs.Put(orig)
	require.NoError(t, err)

	fetched, err := bs.Get(orig.Cid())
	require.NoError(t, err)
	require.Equal(t, orig.RawData(), fetched.RawData())
}

func (s *Suite) TestHas(t *testing.T) {
	bs, _ := s.NewBlockstore(t)
	if c, ok := bs.(io.Closer); ok {
		defer func() { require.NoError(t, c.Close()) }()
	}

	orig := blocks.NewBlock([]byte("some data"))

	err := bs.Put(orig)
	require.NoError(t, err)

	ok, err := bs.Has(orig.Cid())
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = bs.Has(blocks.NewBlock([]byte("another thing")).Cid())
	require.NoError(t, err)
	require.False(t, ok)
}

func (s *Suite) TestCidv0v1(t *testing.T) {
	bs, _ := s.NewBlockstore(t)
	if c, ok := bs.(io.Closer); ok {
		defer func() { require.NoError(t, c.Close()) }()
	}

	orig := blocks.NewBlock([]byte("some data"))

	err := bs.Put(orig)
	require.NoError(t, err)

	fetched, err := bs.Get(cid.NewCidV1(cid.DagProtobuf, orig.Cid().Hash()))
	require.NoError(t, err)
	require.Equal(t, orig.RawData(), fetched.RawData())
}

func (s *Suite) TestPutThenGetSizeBlock(t *testing.T) {
	bs, _ := s.NewBlockstore(t)
	if c, ok := bs.(io.Closer); ok {
		defer func() { require.NoError(t, c.Close()) }()
	}

	block := blocks.NewBlock([]byte("some data"))
	missingBlock := blocks.NewBlock([]byte("missingBlock"))
	emptyBlock := blocks.NewBlock([]byte{})

	err := bs.Put(block)
	require.NoError(t, err)

	blockSize, err := bs.GetSize(block.Cid())
	require.NoError(t, err)
	require.Len(t, block.RawData(), blockSize)

	err = bs.Put(emptyBlock)
	require.NoError(t, err)

	emptySize, err := bs.GetSize(emptyBlock.Cid())
	require.NoError(t, err)
	require.Zero(t, emptySize)

	missingSize, err := bs.GetSize(missingBlock.Cid())
	require.Equal(t, blockstore.ErrNotFound, err)
	require.Equal(t, -1, missingSize)
}

func (s *Suite) TestAllKeysSimple(t *testing.T) {
	bs, _ := s.NewBlockstore(t)
	if c, ok := bs.(io.Closer); ok {
		defer func() { require.NoError(t, c.Close()) }()
	}

	keys := insertBlocks(t, bs, 100)

	ctx := context.Background()
	ch, err := bs.AllKeysChan(ctx)
	require.NoError(t, err)
	actual := collect(ch)

	require.ElementsMatch(t, keys, actual)
}

func (s *Suite) TestAllKeysRespectsContext(t *testing.T) {
	bs, _ := s.NewBlockstore(t)
	if c, ok := bs.(io.Closer); ok {
		defer func() { require.NoError(t, c.Close()) }()
	}

	_ = insertBlocks(t, bs, 100)

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := bs.AllKeysChan(ctx)
	require.NoError(t, err)

	// consume 2, then cancel context.
	v, ok := <-ch
	require.NotEqual(t, cid.Undef, v)
	require.True(t, ok)

	v, ok = <-ch
	require.NotEqual(t, cid.Undef, v)
	require.True(t, ok)

	cancel()

	v, ok = <-ch
	require.Equal(t, cid.Undef, v)
	require.False(t, ok)
}

func (s *Suite) TestDoubleClose(t *testing.T) {
	bs, _ := s.NewBlockstore(t)
	c, ok := bs.(io.Closer)
	if !ok {
		t.SkipNow()
	}
	require.NoError(t, c.Close())
	require.NoError(t, c.Close())
}

func (s *Suite) TestReopenPutGet(t *testing.T) {
	bs, path := s.NewBlockstore(t)
	c, ok := bs.(io.Closer)
	if !ok {
		t.SkipNow()
	}

	orig := blocks.NewBlock([]byte("some data"))
	err := bs.Put(orig)
	require.NoError(t, err)

	err = c.Close()
	require.NoError(t, err)

	bs, err = s.OpenBlockstore(t, path)
	require.NoError(t, err)

	fetched, err := bs.Get(orig.Cid())
	require.NoError(t, err)
	require.Equal(t, orig.RawData(), fetched.RawData())

	err = bs.(io.Closer).Close()
	require.NoError(t, err)
}

func (s *Suite) TestPutMany(t *testing.T) {
	bs, _ := s.NewBlockstore(t)
	if c, ok := bs.(io.Closer); ok {
		defer func() { require.NoError(t, c.Close()) }()
	}

	blks := []blocks.Block{
		blocks.NewBlock([]byte("foo1")),
		blocks.NewBlock([]byte("foo2")),
		blocks.NewBlock([]byte("foo3")),
	}
	err := bs.PutMany(blks)
	require.NoError(t, err)

	for _, blk := range blks {
		fetched, err := bs.Get(blk.Cid())
		require.NoError(t, err)
		require.Equal(t, blk.RawData(), fetched.RawData())

		ok, err := bs.Has(blk.Cid())
		require.NoError(t, err)
		require.True(t, ok)
	}

	ch, err := bs.AllKeysChan(context.Background())
	require.NoError(t, err)

	cids := collect(ch)
	require.Len(t, cids, 3)
}

func (s *Suite) TestDelete(t *testing.T) {
	bs, _ := s.NewBlockstore(t)
	if c, ok := bs.(io.Closer); ok {
		defer func() { require.NoError(t, c.Close()) }()
	}

	blks := []blocks.Block{
		blocks.NewBlock([]byte("foo1")),
		blocks.NewBlock([]byte("foo2")),
		blocks.NewBlock([]byte("foo3")),
	}
	err := bs.PutMany(blks)
	require.NoError(t, err)

	err = bs.DeleteBlock(blks[1].Cid())
	require.NoError(t, err)

	ch, err := bs.AllKeysChan(context.Background())
	require.NoError(t, err)

	cids := collect(ch)
	require.Len(t, cids, 2)
	require.ElementsMatch(t, cids, []cid.Cid{
		cid.NewCidV1(cid.Raw, blks[0].Cid().Hash()),
		cid.NewCidV1(cid.Raw, blks[2].Cid().Hash()),
	})

	has, err := bs.Has(blks[1].Cid())
	require.NoError(t, err)
	require.False(t, has)

}

func insertBlocks(t *testing.T, bs blockstore.Blockstore, count int) []cid.Cid {
	keys := make([]cid.Cid, count)
	for i := 0; i < count; i++ {
		block := blocks.NewBlock([]byte(fmt.Sprintf("some data %d", i)))
		err := bs.Put(block)
		require.NoError(t, err)
		// NewBlock assigns a CIDv0; we convert it to CIDv1 because that's what
		// the store returns.
		keys[i] = cid.NewCidV1(cid.Raw, block.Multihash())
	}
	return keys
}

func collect(ch <-chan cid.Cid) []cid.Cid {
	var keys []cid.Cid
	for k := range ch {
		keys = append(keys, k)
	}
	return keys
}
