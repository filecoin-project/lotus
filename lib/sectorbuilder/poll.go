package sectorbuilder

import (
	"context"
	"time"

	"github.com/prometheus/common/log"
)

// TODO: really need to get a callbacks API from the rust-sectorbuilder
func (sb *SectorBuilder) pollForSealedSectors(ctx context.Context) {
	watching := make(map[uint64]bool)

	staged, err := sb.GetAllStagedSectors()
	if err != nil {
		// TODO: this is probably worth shutting the miner down over until we
		// have better recovery mechanisms
		log.Errorf("failed to get staged sectors: %s", err)
	}
	for _, s := range staged {
		watching[s.SectorID] = true
	}

	tick := time.Tick(time.Second * 30)
	for {
		select {
		case <-tick:
			log.Info("polling for sealed sectors...")

			// add new staged sectors to watch list
			staged, err := sb.GetAllStagedSectors()
			if err != nil {
				log.Errorf("in loop: failed to get staged sectors: %s", err)
				continue
			}
			for _, s := range staged {
				watching[s.SectorID] = true
			}

			for s := range watching {
				status, err := sb.SealStatus(s)
				if err != nil {
					log.Errorf("getting seal status: %s", err)
					continue
				}

				if status.SealStatusCode == 0 { // constant pls, zero implies the last step?
					delete(watching, s)
					sb.sschan <- status
				}
			}
		case <-ctx.Done():
			close(sb.sschan)
			return
		}
	}
}
