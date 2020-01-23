package sealing

import (
	"fmt"
	"time"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/statemachine"
)

const minRetryTime = 1 * time.Minute

func failedCooldown(ctx statemachine.Context, sector SectorInfo) error {
	retryStart := time.Unix(int64(sector.Log[len(sector.Log)-1].Timestamp), 0).Add(minRetryTime)
	if len(sector.Log) > 0 && !time.Now().After(retryStart) {
		log.Infof("%s(%d), waiting %s before retrying", api.SectorStates[sector.State], time.Until(retryStart))
		select {
		case <-time.After(time.Until(retryStart)):
		case <-ctx.Context().Done():
			return ctx.Context().Err()
		}
	}

	return nil
}

func (m *Sealing) handleSealFailed(ctx statemachine.Context, sector SectorInfo) error {
	// TODO: <Remove this section after we can re-precommit>

	act, err := m.api.StateGetActor(ctx.Context(), m.maddr, nil)
	if err != nil {
		log.Errorf("handleSealFailed(%d): temp error: %+v", sector.SectorID, err)
		return nil
	}

	st, err := m.api.StateReadState(ctx.Context(), act, nil)
	if err != nil {
		log.Errorf("handleSealFailed(%d): temp error: %+v", sector.SectorID, err)
		return nil
	}

	_, found := st.State.(map[string]interface{})["PreCommittedSectors"].(map[string]interface{})[fmt.Sprint(sector.SectorID)]
	if found {
		// TODO: If not expired yet, we can just try reusing sealticket
		log.Errorf("sector found in miner preseal array: %+v", sector.SectorID, err)
		return nil
	}
	// </>

	if err := failedCooldown(ctx, sector); err != nil {
		return err
	}

	return ctx.Send(SectorRetrySeal{})
}
