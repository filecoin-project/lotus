package chain

import (
	"sort"
	"sync"
	"time"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	lru "github.com/hashicorp/golang-lru"
	"github.com/libp2p/go-libp2p-core/peer"
)

type blockReceiptTracker struct {
	lk sync.Mutex

	// using an LRU cache because i don't want to handle all the edge cases for
	// manual cleanup and maintenance of a fixed size set
	cache *lru.Cache
}

type peerSet struct {
	peers map[peer.ID]time.Time
}

func newBlockReceiptTracker() *blockReceiptTracker {
	c, _ := lru.New(512)
	return &blockReceiptTracker{
		cache: c,
	}
}

func (brt *blockReceiptTracker) Add(p peer.ID, ts *types.TipSet) {
	brt.lk.Lock()
	defer brt.lk.Unlock()

	val, ok := brt.cache.Get(ts.Key())
	if !ok {
		pset := &peerSet{
			peers: map[peer.ID]time.Time{
				p: build.Clock.Now(),
			},
		}
		brt.cache.Add(ts.Key(), pset)
		return
	}

	val.(*peerSet).peers[p] = build.Clock.Now()
}

func (brt *blockReceiptTracker) GetPeers(ts *types.TipSet) []peer.ID {
	brt.lk.Lock()
	defer brt.lk.Unlock()

	val, ok := brt.cache.Get(ts.Key())
	if !ok {
		return nil
	}

	ps := val.(*peerSet)

	out := make([]peer.ID, 0, len(ps.peers))
	for p := range ps.peers {
		out = append(out, p)
	}

	sort.Slice(out, func(i, j int) bool {
		return ps.peers[out[i]].Before(ps.peers[out[j]])
	})

	return out
}
