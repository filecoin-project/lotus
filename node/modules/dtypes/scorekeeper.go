package dtypes

import (
	"sync"

	peer "github.com/libp2p/go-libp2p-peer"
)

type ScoreKeeper struct {
	lk     sync.Mutex
	scores map[peer.ID]float64
}

func (sk *ScoreKeeper) Update(scores map[peer.ID]float64) {
	sk.lk.Lock()
	sk.scores = scores
	sk.lk.Unlock()
}

func (sk *ScoreKeeper) Get() map[peer.ID]float64 {
	sk.lk.Lock()
	defer sk.lk.Unlock()
	return sk.scores
}
