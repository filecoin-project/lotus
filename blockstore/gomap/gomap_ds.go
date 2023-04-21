package gomapbs

import (
	"context"
	"sync"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-libipfs/blocks"
	logging "github.com/ipfs/go-log/v2"

	"github.com/snissn/gomap"
)

var log = logging.Logger("gomapbs")

type GomapDatastore struct {
	gmap *gomap.Hashmap
	lock sync.RWMutex
}

func NewGomapDS(folder string) (*GomapDatastore, error) {
	var h gomap.Hashmap
	h.New(folder)
	gmds := GomapDatastore{gmap: &h}
	return &gmds, nil
}

func toKey(key cid.Cid) string {

	return string(key.Bytes()) //todo zero copy?
}

func (gmds *GomapDatastore) Put(ctx context.Context, block blocks.Block) error {
	gmds.lock.Lock()
	defer gmds.lock.Unlock()
	gmds.gmap.Add(toKey(block.Cid()), string(block.RawData()))
	//todo less copy and return err
	return nil
}

func (gmds *GomapDatastore) PutMany(ctx context.Context, blocks []blocks.Block) error {
	for _, block := range blocks {
		//Put holds lock
		gmds.Put(ctx, block)
	}
	return nil
}

func (gmds *GomapDatastore) Delete(ctx context.Context, key cid.Cid) error {
	//not yet implemented
	return nil
}

func (gmds *GomapDatastore) Get(ctx context.Context, key cid.Cid) (blocks.Block, error) {
	gmds.lock.RLock()
	defer gmds.lock.RUnlock()
	val, err := gmds.gmap.Get(toKey(key))
	if err != nil {
		//todo check error
		return nil, datastore.ErrNotFound
	}

	return blocks.NewBlockWithCid([]byte(val), key)

}

func (gmds *GomapDatastore) Has(ctx context.Context, key cid.Cid) (bool, error) {
	_, err := gmds.Get(ctx, key)
	return err != datastore.ErrNotFound, nil
	//todo check error type
}

// AllKeysChan implements Blockstore.AllKeysChan.
func (gmds *GomapDatastore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return nil, nil
	//todo
}

func (gmds *GomapDatastore) DeleteBlock(ctx context.Context, cid cid.Cid) error {
	return nil
	//todo
}

func (gmds *GomapDatastore) GetSize(ctx context.Context, cid cid.Cid) (int, error) {
	return 0, nil
	//todo
}

func (gmds *GomapDatastore) HashOnRead(_ bool) {
	log.Warnf("called HashOnRead on badger blockstore; function not supported; ignoring")
}
