package processor

import (
	"context"
	"strings"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/ipfs/go-cid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apibstore"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/events/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	cw_util "github.com/filecoin-project/lotus/cmd/lotus-chainwatch/util"
)

func (p *Processor) setupMiners() error {
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`

create table if not exists miner_info
(
	miner_id text not null,
	owner_addr text not null,
	worker_addr text not null,
	peer_id text,
	sector_size text not null,
	
	constraint miner_info_pk
		primary key (miner_id)
);

create table if not exists sector_precommit_info
(
    miner_id text not null,
    sector_id bigint not null,
    sealed_cid text not null,
    state_root text not null,
    
    seal_rand_epoch bigint not null,
    expiration_epoch bigint not null,
    
    precommit_deposit text not null,
    precommit_epoch bigint not null,
    deal_weight text not null,
    verified_deal_weight text not null,
    
    
    is_replace_capacity bool not null,
    replace_sector_deadline bigint,
    replace_sector_partition bigint,
    replace_sector_number bigint,
    
    unique (miner_id, sector_id),
    
    constraint sector_precommit_info_pk
		primary key (miner_id, sector_id, sealed_cid)
    
);

create table if not exists sector_info
(
    miner_id text not null,
    sector_id bigint not null,
    sealed_cid text not null,
    state_root text not null,
    
    activation_epoch bigint not null,
    expiration_epoch bigint not null,
    
    deal_weight text not null,
    verified_deal_weight text not null,
    
    initial_pledge text not null,
	expected_day_reward text not null,
	expected_storage_pledge text not null,
    
    constraint sector_info_pk
		primary key (miner_id, sector_id, sealed_cid)
);

/*
* captures miner-specific power state for any given stateroot
*/
create table if not exists miner_power
(
	miner_id text not null,
	state_root text not null,
	raw_bytes_power text not null,
	quality_adjusted_power text not null,
	constraint miner_power_pk
		primary key (miner_id, state_root)
);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'miner_sector_event_type') THEN
        CREATE TYPE miner_sector_event_type AS ENUM
        (
            'PRECOMMIT_ADDED', 'PRECOMMIT_EXPIRED', 'COMMIT_CAPACITY_ADDED', 'SECTOR_ADDED',
            'SECTOR_EXTENDED', 'SECTOR_EXPIRED', 'SECTOR_FAULTED', 'SECTOR_RECOVERING', 'SECTOR_RECOVERED', 'SECTOR_TERMINATED'
        );
    END IF;
END$$;

create table if not exists miner_sector_events
(
    miner_id text not null,
    sector_id bigint not null,
    state_root text not null,
    event miner_sector_event_type not null,
    
	constraint miner_sector_events_pk
		primary key (sector_id, event, miner_id, state_root)
);

`); err != nil {
		return err
	}

	return tx.Commit()
}

type SectorLifecycleEvent string

const (
	PreCommitAdded   = "PRECOMMIT_ADDED"
	PreCommitExpired = "PRECOMMIT_EXPIRED"

	CommitCapacityAdded = "COMMIT_CAPACITY_ADDED"

	SectorAdded      = "SECTOR_ADDED"
	SectorExpired    = "SECTOR_EXPIRED"
	SectorExtended   = "SECTOR_EXTENDED"
	SectorFaulted    = "SECTOR_FAULTED"
	SectorRecovering = "SECTOR_RECOVERING"
	SectorRecovered  = "SECTOR_RECOVERED"
	SectorTerminated = "SECTOR_TERMINATED"
)

type MinerSectorsEvent struct {
	MinerID   address.Address
	SectorIDs []uint64
	StateRoot cid.Cid
	Event     SectorLifecycleEvent
}

type SectorDealEvent struct {
	MinerID  address.Address
	SectorID uint64
	DealIDs  []abi.DealID
}

type PartitionStatus struct {
	Terminated bitfield.BitField
	Expired    bitfield.BitField
	Faulted    bitfield.BitField
	InRecovery bitfield.BitField
	Recovered  bitfield.BitField
}

type minerActorInfo struct {
	common actorInfo

	state miner.State

	// tracked by power actor
	rawPower big.Int
	qalPower big.Int
}

func (p *Processor) HandleMinerChanges(ctx context.Context, minerTips ActorTips) error {
	minerChanges, err := p.processMiners(ctx, minerTips)
	if err != nil {
		log.Fatalw("Failed to process miner actors", "error", err)
	}

	if err := p.persistMiners(ctx, minerChanges); err != nil {
		log.Fatalw("Failed to persist miner actors", "error", err)
	}

	return nil
}

func (p *Processor) processMiners(ctx context.Context, minerTips map[types.TipSetKey][]actorInfo) ([]minerActorInfo, error) {
	start := time.Now()
	defer func() {
		log.Debugw("Processed Miners", "duration", time.Since(start).String())
	}()

	stor := store.ActorStore(ctx, apibstore.NewAPIBlockstore(p.node))

	var out []minerActorInfo
	// TODO add parallel calls if this becomes slow
	for tipset, miners := range minerTips {
		// get the power actors claims map
		powerState, err := getPowerActorState(ctx, p.node, tipset)
		if err != nil {
			return nil, err
		}

		// Get miner raw and quality power
		for _, act := range miners {
			var mi minerActorInfo
			mi.common = act

			// get miner claim from power actors claim map and store if found, else the miner had no claim at
			// this tipset
			claim, found, err := powerState.MinerPower(act.addr)
			if err != nil {
				return nil, err
			}
			if found {
				mi.qalPower = claim.QualityAdjPower
				mi.rawPower = claim.RawBytePower
			}

			// Get the miner state
			mas, err := miner.Load(stor, &act.act)
			if err != nil {
				log.Warnw("failed to find miner actor state", "address", act.addr, "error", err)
				continue
			}
			mi.state = mas
			out = append(out, mi)
		}
	}
	return out, nil
}

func (p *Processor) persistMiners(ctx context.Context, miners []minerActorInfo) error {
	start := time.Now()
	defer func() {
		log.Debugw("Persisted Miners", "duration", time.Since(start).String())
	}()

	grp, _ := errgroup.WithContext(ctx)

	grp.Go(func() error {
		if err := p.storeMinersPower(miners); err != nil {
			return err
		}
		return nil
	})

	grp.Go(func() error {
		if err := p.storeMinersActorInfoState(ctx, miners); err != nil {
			return err
		}
		return nil
	})

	// 8 is arbitrary, idk what a good value here is.
	preCommitEvents := make(chan *MinerSectorsEvent, 8)
	sectorEvents := make(chan *MinerSectorsEvent, 8)
	partitionEvents := make(chan *MinerSectorsEvent, 8)
	dealEvents := make(chan *SectorDealEvent, 8)

	grp.Go(func() error {
		return p.storePreCommitDealInfo(dealEvents)
	})

	grp.Go(func() error {
		return p.storeMinerSectorEvents(ctx, sectorEvents, preCommitEvents, partitionEvents)
	})

	grp.Go(func() error {
		defer func() {
			close(preCommitEvents)
			close(dealEvents)
		}()
		return p.storeMinerPreCommitInfo(ctx, miners, preCommitEvents, dealEvents)
	})

	grp.Go(func() error {
		defer close(sectorEvents)
		return p.storeMinerSectorInfo(ctx, miners, sectorEvents)
	})

	grp.Go(func() error {
		defer close(partitionEvents)
		return p.getMinerPartitionsDifferences(ctx, miners, partitionEvents)
	})

	return grp.Wait()
}

func (p *Processor) storeMinerPreCommitInfo(ctx context.Context, miners []minerActorInfo, sectorEvents chan<- *MinerSectorsEvent, sectorDeals chan<- *SectorDealEvent) error {
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`create temp table spi (like sector_precommit_info excluding constraints) on commit drop;`); err != nil {
		return xerrors.Errorf("Failed to create temp table for sector_precommit_info: %w", err)
	}

	stmt, err := tx.Prepare(`copy spi (miner_id, sector_id, sealed_cid, state_root, seal_rand_epoch, expiration_epoch, precommit_deposit, precommit_epoch, deal_weight, verified_deal_weight, is_replace_capacity, replace_sector_deadline, replace_sector_partition, replace_sector_number) from STDIN`)

	if err != nil {
		return xerrors.Errorf("Failed to prepare miner precommit info statement: %w", err)
	}

	grp, _ := errgroup.WithContext(ctx)
	for _, m := range miners {
		m := m
		grp.Go(func() error {
			changes, err := p.getMinerPreCommitChanges(ctx, m)
			if err != nil {
				if strings.Contains(err.Error(), types.ErrActorNotFound.Error()) {
					return nil
				}
				return err
			}
			if changes == nil {
				return nil
			}

			preCommitAdded := make([]uint64, len(changes.Added))
			for i, added := range changes.Added {
				if len(added.Info.DealIDs) > 0 {
					sectorDeals <- &SectorDealEvent{
						MinerID:  m.common.addr,
						SectorID: uint64(added.Info.SectorNumber),
						DealIDs:  added.Info.DealIDs,
					}
				}
				if added.Info.ReplaceCapacity {
					if _, err := stmt.Exec(
						m.common.addr.String(),
						added.Info.SectorNumber,
						added.Info.SealedCID.String(),
						m.common.stateroot.String(),
						added.Info.SealRandEpoch,
						added.Info.Expiration,
						added.PreCommitDeposit.String(),
						added.PreCommitEpoch,
						added.DealWeight.String(),
						added.VerifiedDealWeight.String(),
						added.Info.ReplaceCapacity,
						added.Info.ReplaceSectorDeadline,
						added.Info.ReplaceSectorPartition,
						added.Info.ReplaceSectorNumber,
					); err != nil {
						return err
					}
				} else {
					if _, err := stmt.Exec(
						m.common.addr.String(),
						added.Info.SectorNumber,
						added.Info.SealedCID.String(),
						m.common.stateroot.String(),
						added.Info.SealRandEpoch,
						added.Info.Expiration,
						added.PreCommitDeposit.String(),
						added.PreCommitEpoch,
						added.DealWeight.String(),
						added.VerifiedDealWeight.String(),
						added.Info.ReplaceCapacity,
						nil, // replace deadline
						nil, // replace partition
						nil, // replace sector
					); err != nil {
						return err
					}

				}
				preCommitAdded[i] = uint64(added.Info.SectorNumber)
			}
			if len(preCommitAdded) > 0 {
				sectorEvents <- &MinerSectorsEvent{
					MinerID:   m.common.addr,
					StateRoot: m.common.stateroot,
					SectorIDs: preCommitAdded,
					Event:     PreCommitAdded,
				}
			}
			var preCommitExpired []uint64
			for _, removed := range changes.Removed {
				// TODO: we can optimize this to not load the AMT every time, if necessary.
				si, err := m.state.GetSector(removed.Info.SectorNumber)
				if err != nil {
					return err
				}
				if si == nil {
					preCommitExpired = append(preCommitExpired, uint64(removed.Info.SectorNumber))
				}
			}
			if len(preCommitExpired) > 0 {
				sectorEvents <- &MinerSectorsEvent{
					MinerID:   m.common.addr,
					StateRoot: m.common.stateroot,
					SectorIDs: preCommitExpired,
					Event:     PreCommitExpired,
				}
			}
			return nil
		})
	}
	if err := grp.Wait(); err != nil {
		return err
	}

	if err := stmt.Close(); err != nil {
		return xerrors.Errorf("Failed to close sector precommit info statement: %w", err)
	}

	if _, err := tx.Exec(`insert into sector_precommit_info select * from spi on conflict do nothing`); err != nil {
		return xerrors.Errorf("Failed to insert into sector precommit info table: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("Failed to commit sector precommit info: %w", err)
	}
	return nil
}

func (p *Processor) storeMinerSectorInfo(ctx context.Context, miners []minerActorInfo, events chan<- *MinerSectorsEvent) error {
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`create temp table si (like sector_info excluding constraints) on commit drop;`); err != nil {
		return xerrors.Errorf("Failed to create temp table for sector_: %w", err)
	}

	stmt, err := tx.Prepare(`copy si (miner_id, sector_id, sealed_cid, state_root, activation_epoch, expiration_epoch, deal_weight, verified_deal_weight, initial_pledge, expected_day_reward, expected_storage_pledge) from STDIN`)
	if err != nil {
		return xerrors.Errorf("Failed to prepare miner sector info statement: %w", err)
	}

	grp, _ := errgroup.WithContext(ctx)
	for _, m := range miners {
		m := m
		grp.Go(func() error {
			changes, err := p.getMinerSectorChanges(ctx, m)
			if err != nil {
				if strings.Contains(err.Error(), types.ErrActorNotFound.Error()) {
					return nil
				}
				return err
			}
			if changes == nil {
				return nil
			}
			var sectorsAdded []uint64
			var ccAdded []uint64
			var extended []uint64
			for _, added := range changes.Added {
				// add the sector to the table
				if _, err := stmt.Exec(
					m.common.addr.String(),
					added.SectorNumber,
					added.SealedCID.String(),
					m.common.stateroot.String(),
					added.Activation.String(),
					added.Expiration.String(),
					added.DealWeight.String(),
					added.VerifiedDealWeight.String(),
					added.InitialPledge.String(),
					added.ExpectedDayReward.String(),
					added.ExpectedStoragePledge.String(),
				); err != nil {
					log.Errorw("writing miner sector changes statement", "error", err.Error())
				}
				if len(added.DealIDs) == 0 {
					ccAdded = append(ccAdded, uint64(added.SectorNumber))
				} else {
					sectorsAdded = append(sectorsAdded, uint64(added.SectorNumber))
				}
			}

			for _, mod := range changes.Extended {
				extended = append(extended, uint64(mod.To.SectorNumber))
			}

			events <- &MinerSectorsEvent{
				MinerID:   m.common.addr,
				StateRoot: m.common.stateroot,
				SectorIDs: ccAdded,
				Event:     CommitCapacityAdded,
			}
			events <- &MinerSectorsEvent{
				MinerID:   m.common.addr,
				StateRoot: m.common.stateroot,
				SectorIDs: sectorsAdded,
				Event:     SectorAdded,
			}
			events <- &MinerSectorsEvent{
				MinerID:   m.common.addr,
				StateRoot: m.common.stateroot,
				SectorIDs: extended,
				Event:     SectorExtended,
			}
			return nil
		})
	}

	if err := grp.Wait(); err != nil {
		return err
	}

	if err := stmt.Close(); err != nil {
		return xerrors.Errorf("Failed to close sector info statement: %w", err)
	}

	if _, err := tx.Exec(`insert into sector_info select * from si on conflict do nothing`); err != nil {
		return xerrors.Errorf("Failed to insert into sector info table: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("Failed to commit sector info: %w", err)
	}
	return nil

}

func (p *Processor) getMinerPartitionsDifferences(ctx context.Context, miners []minerActorInfo, events chan<- *MinerSectorsEvent) error {
	grp, ctx := errgroup.WithContext(ctx)
	for _, m := range miners {
		m := m
		grp.Go(func() error {
			if err := p.diffMinerPartitions(ctx, m, events); err != nil {
				if strings.Contains(err.Error(), types.ErrActorNotFound.Error()) {
					return nil
				}
				return err
			}
			return nil
		})
	}
	return grp.Wait()
}

func (p *Processor) storeMinerSectorEvents(ctx context.Context, sectorEvents, preCommitEvents, partitionEvents <-chan *MinerSectorsEvent) error {
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`create temp table mse (like miner_sector_events excluding constraints) on commit drop;`); err != nil {
		return xerrors.Errorf("Failed to create temp table for sector_: %w", err)
	}

	stmt, err := tx.Prepare(`copy mse (miner_id, sector_id, event, state_root) from STDIN`)
	if err != nil {
		return xerrors.Errorf("Failed to prepare miner sector info statement: %w", err)
	}

	grp, ctx := errgroup.WithContext(ctx)
	grp.Go(func() error {
		innerGrp, _ := errgroup.WithContext(ctx)
		for mse := range sectorEvents {
			mse := mse
			innerGrp.Go(func() error {
				for _, sid := range mse.SectorIDs {
					if _, err := stmt.Exec(
						mse.MinerID.String(),
						sid,
						mse.Event,
						mse.StateRoot.String(),
					); err != nil {
						return err
					}
				}
				return nil
			})
		}
		return innerGrp.Wait()
	})

	grp.Go(func() error {
		innerGrp, _ := errgroup.WithContext(ctx)
		for mse := range preCommitEvents {
			mse := mse
			innerGrp.Go(func() error {
				for _, sid := range mse.SectorIDs {
					if _, err := stmt.Exec(
						mse.MinerID.String(),
						sid,
						mse.Event,
						mse.StateRoot.String(),
					); err != nil {
						return err
					}
				}
				return nil
			})
		}
		return innerGrp.Wait()
	})

	grp.Go(func() error {
		innerGrp, _ := errgroup.WithContext(ctx)
		for mse := range partitionEvents {
			mse := mse
			grp.Go(func() error {
				for _, sid := range mse.SectorIDs {
					if _, err := stmt.Exec(
						mse.MinerID.String(),
						sid,
						mse.Event,
						mse.StateRoot.String(),
					); err != nil {
						return err
					}
				}
				return nil
			})
		}
		return innerGrp.Wait()
	})

	if err := grp.Wait(); err != nil {
		return err
	}

	if err := stmt.Close(); err != nil {
		return xerrors.Errorf("Failed to close sector event statement: %w", err)
	}

	if _, err := tx.Exec(`insert into miner_sector_events select * from mse on conflict do nothing`); err != nil {
		return xerrors.Errorf("Failed to insert into sector event table: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("Failed to commit sector events: %w", err)
	}
	return nil
}

func (p *Processor) getMinerStateAt(ctx context.Context, maddr address.Address, tskey types.TipSetKey) (miner.State, error) {
	prevActor, err := p.node.StateGetActor(ctx, maddr, tskey)
	if err != nil {
		return nil, err
	}
	return miner.Load(store.ActorStore(ctx, apibstore.NewAPIBlockstore(p.node)), prevActor)
}

func (p *Processor) getMinerPreCommitChanges(ctx context.Context, m minerActorInfo) (*miner.PreCommitChanges, error) {
	pred := state.NewStatePredicates(p.node)
	changed, val, err := pred.OnMinerActorChange(m.common.addr, pred.OnMinerPreCommitChange())(ctx, m.common.parentTsKey, m.common.tsKey)
	if err != nil {
		return nil, xerrors.Errorf("Failed to diff miner precommit amt: %w", err)
	}
	if !changed {
		return nil, nil
	}
	out := val.(*miner.PreCommitChanges)
	return out, nil
}

func (p *Processor) getMinerSectorChanges(ctx context.Context, m minerActorInfo) (*miner.SectorChanges, error) {
	pred := state.NewStatePredicates(p.node)
	changed, val, err := pred.OnMinerActorChange(m.common.addr, pred.OnMinerSectorChange())(ctx, m.common.parentTsKey, m.common.tsKey)
	if err != nil {
		return nil, xerrors.Errorf("Failed to diff miner sectors amt: %w", err)
	}
	if !changed {
		return nil, nil
	}
	out := val.(*miner.SectorChanges)
	return out, nil
}

func (p *Processor) diffMinerPartitions(ctx context.Context, m minerActorInfo, events chan<- *MinerSectorsEvent) error {
	prevMiner, err := p.getMinerStateAt(ctx, m.common.addr, m.common.parentTsKey)
	if err != nil {
		return err
	}
	curMiner := m.state
	dc, err := prevMiner.DeadlinesChanged(curMiner)
	if err != nil {
		return err
	}
	if !dc {
		return nil
	}
	panic("TODO")

	// FIXME: This code doesn't work.
	// 1. We need to diff all deadlines, not just the "current" deadline.
	// 2. We need to handle the case where we _add_ a partition. (i.e.,
	// where len(newPartitions) != len(oldPartitions).
	/*

			// NOTE: If we change the number of deadlines in an upgrade, this will
			// break.

			// load the old deadline
			prevDls, err := prevMiner.LoadDeadlines(p.ctxStore)
			if err != nil {
				return err
			}
			var prevDl miner.Deadline
			if err := p.ctxStore.Get(ctx, prevDls.Due[dlIdx], &prevDl); err != nil {
				return err
			}

			prevPartitions, err := prevDl.PartitionsArray(p.ctxStore)
			if err != nil {
				return err
			}

			// load the new deadline
			curDls, err := curMiner.LoadDeadlines(p.ctxStore)
			if err != nil {
				return err
			}

			var curDl miner.Deadline
			if err := p.ctxStore.Get(ctx, curDls.Due[dlIdx], &curDl); err != nil {
				return err
			}

			curPartitions, err := curDl.PartitionsArray(p.ctxStore)
			if err != nil {
				return err
			}

			// TODO this can be optimized by inspecting the miner state for partitions that have changed and only inspecting those.
			var prevPart miner.Partition
			if err := prevPartitions.ForEach(&prevPart, func(i int64) error {
				var curPart miner.Partition
				if found, err := curPartitions.Get(uint64(i), &curPart); err != nil {
					return err
				} else if !found {
					log.Fatal("I don't know what this means, are partitions ever removed?")
				}
				partitionDiff, err := p.diffPartition(prevPart, curPart)
				if err != nil {
					return err
				}

				recovered, err := partitionDiff.Recovered.All(miner.SectorsMax)
				if err != nil {
					return err
				}
				events <- &MinerSectorsEvent{
					MinerID:   m.common.addr,
					StateRoot: m.common.stateroot,
					SectorIDs: recovered,
					Event:     SectorRecovered,
				}
				inRecovery, err := partitionDiff.InRecovery.All(miner.SectorsMax)
				if err != nil {
					return err
				}
				events <- &MinerSectorsEvent{
					MinerID:   m.common.addr,
					StateRoot: m.common.stateroot,
					SectorIDs: inRecovery,
					Event:     SectorRecovering,
				}
				faulted, err := partitionDiff.Faulted.All(miner.SectorsMax)
				if err != nil {
					return err
				}
				events <- &MinerSectorsEvent{
					MinerID:   m.common.addr,
					StateRoot: m.common.stateroot,
					SectorIDs: faulted,
					Event:     SectorFaulted,
				}
				terminated, err := partitionDiff.Terminated.All(miner.SectorsMax)
				if err != nil {
					return err
				}
				events <- &MinerSectorsEvent{
					MinerID:   m.common.addr,
					StateRoot: m.common.stateroot,
					SectorIDs: terminated,
					Event:     SectorTerminated,
				}
				expired, err := partitionDiff.Expired.All(miner.SectorsMax)
				if err != nil {
					return err
				}
				events <- &MinerSectorsEvent{
					MinerID:   m.common.addr,
					StateRoot: m.common.stateroot,
					SectorIDs: expired,
					Event:     SectorExpired,
				}

				return nil
			}); err != nil {
				return err
			}

		return nil
	*/
}

func (p *Processor) diffPartition(prevPart, curPart miner.Partition) (*PartitionStatus, error) {
	prevLiveSectors, err := prevPart.LiveSectors()
	if err != nil {
		return nil, err
	}
	curLiveSectors, err := curPart.LiveSectors()
	if err != nil {
		return nil, err
	}

	removedSectors, err := bitfield.SubtractBitField(prevLiveSectors, curLiveSectors)
	if err != nil {
		return nil, err
	}

	prevRecoveries, err := prevPart.RecoveringSectors()
	if err != nil {
		return nil, err
	}

	curRecoveries, err := curPart.RecoveringSectors()
	if err != nil {
		return nil, err
	}

	newRecoveries, err := bitfield.SubtractBitField(curRecoveries, prevRecoveries)
	if err != nil {
		return nil, err
	}

	prevFaults, err := prevPart.FaultySectors()
	if err != nil {
		return nil, err
	}

	curFaults, err := curPart.FaultySectors()
	if err != nil {
		return nil, err
	}

	newFaults, err := bitfield.SubtractBitField(curFaults, prevFaults)
	if err != nil {
		return nil, err
	}

	// all current good sectors
	curActiveSectors, err := curPart.ActiveSectors()
	if err != nil {
		return nil, err
	}

	// sectors that were previously fault and are now currently active are considered recovered.
	recovered, err := bitfield.IntersectBitField(prevFaults, curActiveSectors)
	if err != nil {
		return nil, err
	}

	// TODO: distinguish between "terminated" and "expired" sectors. The
	// previous code here never had a chance of working in the first place,
	// so I'm not going to try to replicate it right now.
	//
	// How? If the sector expires before it should (according to sector
	// info) and it wasn't replaced by a pre-commit deleted in this change
	// set, it was "early terminated".

	return &PartitionStatus{
		Terminated: bitfield.New(),
		Expired:    removedSectors,
		Faulted:    newFaults,
		InRecovery: newRecoveries,
		Recovered:  recovered,
	}, nil
}

func (p *Processor) storeMinersActorInfoState(ctx context.Context, miners []minerActorInfo) error {
	start := time.Now()
	defer func() {
		log.Debugw("Stored Miners Actor State", "duration", time.Since(start).String())
	}()

	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`create temp table mi (like miner_info excluding constraints) on commit drop;`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy mi (miner_id, owner_addr, worker_addr, peer_id, sector_size) from STDIN`)
	if err != nil {
		return err
	}
	for _, m := range miners {
		mi, err := p.node.StateMinerInfo(ctx, m.common.addr, m.common.tsKey)
		if err != nil {
			if strings.Contains(err.Error(), types.ErrActorNotFound.Error()) {
				continue
			} else {
				return err
			}
		}
		var pid string
		if mi.PeerId != nil {
			pid = mi.PeerId.String()
		}
		if _, err := stmt.Exec(
			m.common.addr.String(),
			mi.Owner.String(),
			mi.Worker.String(),
			pid,
			mi.SectorSize.ShortString(),
		); err != nil {
			log.Errorw("failed to store miner state", "state", m.state, "info", m.state.Info, "error", err)
			return xerrors.Errorf("failed to store miner state: %w", err)
		}

	}
	if err := stmt.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`insert into miner_info select * from mi on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	return tx.Commit()
}

func (p *Processor) storePreCommitDealInfo(dealEvents <-chan *SectorDealEvent) error {
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`create temp table mds (like minerid_dealid_sectorid excluding constraints) on commit  drop;`); err != nil {
		return xerrors.Errorf("Failed to create temp table for minerid_dealid_sectorid: %w", err)
	}

	stmt, err := tx.Prepare(`copy mds (deal_id, miner_id, sector_id) from STDIN`)
	if err != nil {
		return xerrors.Errorf("Failed to prepare minerid_dealid_sectorid statement: %w", err)
	}

	for sde := range dealEvents {
		for _, did := range sde.DealIDs {
			if _, err := stmt.Exec(
				uint64(did),
				sde.MinerID.String(),
				sde.SectorID,
			); err != nil {
				return err
			}
		}
	}

	if err := stmt.Close(); err != nil {
		return xerrors.Errorf("Failed to close miner sector deals statement: %w", err)
	}

	if _, err := tx.Exec(`insert into minerid_dealid_sectorid select * from mds on conflict do nothing`); err != nil {
		return xerrors.Errorf("Failed to insert into miner deal sector table: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("Failed to commit miner deal sector table: %w", err)
	}
	return nil

}

func (p *Processor) storeMinersPower(miners []minerActorInfo) error {
	start := time.Now()
	defer func() {
		log.Debugw("Stored Miners Power", "duration", time.Since(start).String())
	}()

	tx, err := p.db.Begin()
	if err != nil {
		return xerrors.Errorf("begin miner_power tx: %w", err)
	}

	if _, err := tx.Exec(`create temp table mp (like miner_power excluding constraints) on commit drop`); err != nil {
		return xerrors.Errorf("prep miner_power temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy mp (miner_id, state_root, raw_bytes_power, quality_adjusted_power) from STDIN`)
	if err != nil {
		return xerrors.Errorf("prepare tmp miner_power: %w", err)
	}

	for _, m := range miners {
		if _, err := stmt.Exec(
			m.common.addr.String(),
			m.common.stateroot.String(),
			m.rawPower.String(),
			m.qalPower.String(),
		); err != nil {
			log.Errorw("failed to store miner power", "miner", m.common.addr, "stateroot", m.common.stateroot, "error", err)
		}
	}

	if err := stmt.Close(); err != nil {
		return xerrors.Errorf("close prepared miner_power: %w", err)
	}

	if _, err := tx.Exec(`insert into miner_power select * from mp on conflict do nothing`); err != nil {
		return xerrors.Errorf("insert miner_power from tmp: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("commit miner_power tx: %w", err)
	}

	return nil

}

// load the power actor state clam as an adt.Map at the tipset `ts`.
func getPowerActorState(ctx context.Context, api api.FullNode, ts types.TipSetKey) (power.State, error) {
	powerActor, err := api.StateGetActor(ctx, power.Address, ts)
	if err != nil {
		return nil, err
	}
	return power.Load(cw_util.NewAPIIpldStore(ctx, api), powerActor)
}
