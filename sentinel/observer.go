package sentinel

import (
	"context"

	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/types"
)

var log = logging.Logger("sentinel-observer")

var _ events.TipSetObserver = (*LoggingTipSetObserver)(nil)

type LoggingTipSetObserver struct {
}

func (o *LoggingTipSetObserver) Apply(ctx context.Context, ts *types.TipSet) error {
	log.Infof("TipSetObserver.Apply(%q)", ts.Key())
	return nil
}

func (o *LoggingTipSetObserver) Revert(ctx context.Context, ts *types.TipSet) error {
	log.Infof("TipSetObserver.Revert(%q)", ts.Key())
	return nil
}
