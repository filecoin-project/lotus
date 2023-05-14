package gomapbs

import (
	"context"
	"fmt"
	"sync"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"

	"github.com/snissn/gomap"
)

var log = logging.Logger("gomapbs")

type GomapDatastore struct {
	gmap *gomap.Hashmap
	lock sync.RWMutex
}

func NewGomapDS(folder string) (*GomapDatastore, error) {
	fmt.Println("NewGomapDS", folder)
	var h gomap.Hashmap
	h.New(folder)
	gmds := GomapDatastore{gmap: &h}
	return &gmds, nil
}

func toKey(key cid.Cid) []byte {

	return key.Bytes() //todo zero copy?
}

func (gmds *GomapDatastore) Put(ctx context.Context, block blocks.Block) error {
	gmds.lock.Lock()
	defer gmds.lock.Unlock()
	gmds.gmap.Add(toKey(block.Cid()), block.RawData())
	//todo less copy and return err
	return nil
}

func (gmds *GomapDatastore) PutMany(ctx context.Context, blocks []blocks.Block) error {
	items := make([]gomap.Item, len(blocks))
	for i, block := range blocks {
		items[i] = gomap.Item{Key: toKey(block.Cid()), Value: block.RawData()}
	}

	gmds.lock.Lock()
	defer gmds.lock.Unlock()

	gmds.gmap.AddMany(items)

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

func (gmds *GomapDatastore) GetSize(ctx context.Context, key cid.Cid) (int, error) {
	val, err := gmds.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	return int(len(val.RawData())), nil
}

func (gmds *GomapDatastore) HashOnRead(_ bool) {
	log.Warnf("called HashOnRead on badger blockstore; function not supported; ignoring")
}
