package ffiwrapper

import (
	"context"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var log = logging.Logger("ffiwrapper")

type ExternPrecommit2 func(ctx context.Context, sector storiface.SectorRef, cache, sealed string, pc1out storiface.PreCommit1Out) (sealedCID cid.Cid, unsealedCID cid.Cid, err error)

type ExternalSealer struct {
	PreCommit2 ExternPrecommit2
}

type Sealer struct {
	sectors SectorProvider

	// externCalls contain overrides for calling alternative sealing logic
	externCalls ExternalSealer

	stopping chan struct{}
}

func (sb *Sealer) Stop() {
	close(sb.stopping)
}
