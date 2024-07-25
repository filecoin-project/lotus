package sync

import (
	"context"
	"time"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/tools/stats/headbuffer"
)

type SyncWaitApi interface {
	SyncState(context.Context) (*api.SyncState, error)
	ChainHead(context.Context) (*types.TipSet, error)
}

// SyncWait returns when ChainHead is within 20 epochs of the expected height
func SyncWait(ctx context.Context, napi SyncWaitApi) error {
	for {
		state, err := napi.SyncState(ctx)
		if err != nil {
			return err
		}

		if len(state.ActiveSyncs) == 0 {
			build.Clock.Sleep(time.Second)
			continue
		}

		head, err := napi.ChainHead(ctx)
		if err != nil {
			return err
		}

		working := -1
		for i, ss := range state.ActiveSyncs {
			switch ss.Stage {
			case api.StageSyncComplete:
			default:
				working = i
			case api.StageIdle:
				// not complete, not actively working
			}
		}

		if working == -1 {
			working = len(state.ActiveSyncs) - 1
		}

		ss := state.ActiveSyncs[working]

		if ss.Base == nil || ss.Target == nil {
			log.Infow(
				"syncing",
				"height", ss.Height,
				"stage", ss.Stage.String(),
			)
		} else {
			log.Infow(
				"syncing",
				"base", ss.Base.Key(),
				"target", ss.Target.Key(),
				"target_height", ss.Target.Height(),
				"height", ss.Height,
				"stage", ss.Stage.String(),
			)
		}

		if build.Clock.Now().Unix()-int64(head.MinTimestamp()) < int64(buildconstants.BlockDelaySecs)*30 {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-build.Clock.After(time.Duration(int64(buildconstants.BlockDelaySecs) * int64(time.Second))):
		}
	}

	return nil
}

type BufferedTipsetChannelApi interface {
	ChainNotify(context.Context) (<-chan []*api.HeadChange, error)
	Version(context.Context) (api.APIVersion, error)
	ChainGetTipSet(context.Context, types.TipSetKey) (*types.TipSet, error)
}

// BufferedTipsetChannel returns an unbuffered channel of tipsets. Buffering occurs internally to handle revert
// ChainNotify changes. The returned channel can output tipsets at the same height twice if a reorg larger the
// provided `size` occurs.
func BufferedTipsetChannel(ctx context.Context, api BufferedTipsetChannelApi, lastHeight abi.ChainEpoch, size int) (<-chan *types.TipSet, error) {
	chmain := make(chan *types.TipSet)

	hb := headbuffer.NewHeadChangeStackBuffer(size)

	notif, err := api.ChainNotify(ctx)
	if err != nil {
		return nil, err
	}

	go func() {
		defer close(chmain)

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case changes, ok := <-notif:
				if !ok {
					return
				}
				for _, change := range changes {
					log.Debugw("head event", "height", change.Val.Height(), "type", change.Type)

					switch change.Type {
					case store.HCCurrent:
						tipsets, err := loadTipsets(ctx, api, change.Val, lastHeight)
						if err != nil {
							log.Info(err)
							return
						}

						for _, tipset := range tipsets {
							chmain <- tipset
						}
					case store.HCApply:
						if out := hb.Push(change); out != nil {
							chmain <- out.Val
						}
					case store.HCRevert:
						hb.Pop()
					}
				}
			case <-ticker.C:
				log.Debug("running health check")

				cctx, cancel := context.WithTimeout(ctx, 5*time.Second)

				if _, err := api.Version(cctx); err != nil {
					log.Error("health check failed")
					cancel()
					return
				}

				cancel()

				log.Debug("node online")
			case <-ctx.Done():
				return
			}
		}
	}()

	return chmain, nil
}

func loadTipsets(ctx context.Context, api BufferedTipsetChannelApi, curr *types.TipSet, lowestHeight abi.ChainEpoch) ([]*types.TipSet, error) {
	log.Infow("loading tipsets", "to_height", lowestHeight, "from_height", curr.Height())
	tipsets := []*types.TipSet{}
	for {
		if curr.Height() == 0 {
			break
		}

		if curr.Height() <= lowestHeight {
			break
		}

		log.Debugw("walking back", "height", curr.Height())
		tipsets = append(tipsets, curr)

		tsk := curr.Parents()
		prev, err := api.ChainGetTipSet(ctx, tsk)
		if err != nil {
			return tipsets, err
		}

		curr = prev
	}

	for i, j := 0, len(tipsets)-1; i < j; i, j = i+1, j-1 {
		tipsets[i], tipsets[j] = tipsets[j], tipsets[i]
	}

	return tipsets, nil
}
