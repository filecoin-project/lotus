package ffiwrapper

import (
	logging "github.com/ipfs/go-log/v2"
	"sync"
)

var log = logging.Logger("ffiwrapper")

type Sealer struct {
	sectors  SectorProvider
	stopping chan struct{}
	postLock sync.Mutex
}

func (sb *Sealer) Stop() {
	close(sb.stopping)
}
