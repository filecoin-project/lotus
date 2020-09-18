package processor

import (
	"bytes"
	"context"
	"strings"
	"time"

	"github.com/ipfs/go-cid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/events/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
)

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

func (p *Processor) storePreCommitDealInfo(ctx context.Context, dealEvents <-chan *SectorDealEvent) error {
	start := time.Now()
	defer log.Debugw("Stored PreCommit Deal Info", "duration", time.Since(start).String())

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

	grp, _ := errgroup.WithContext(ctx)
	for sde := range dealEvents {
		sde := sde
		grp.Go(func() error {
			for _, did := range sde.DealIDs {
				if _, err := stmt.Exec(
					uint64(did),
					sde.MinerID.String(),
					sde.SectorID,
				); err != nil {
					return err
				}
			}
			return nil
		})
	}

	if err := grp.Wait(); err != nil {
		return err
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

func (p *Processor) storeMinerSectorEvents(ctx context.Context, sectorEvents, preCommitEvents, partitionEvents <-chan *MinerSectorsEvent) error {
	start := time.Now()
	defer log.Debugw("Stored Miner Sector Events", "duration", time.Since(start).String())

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

func (p *Processor) storeMinerPreCommitInfo(ctx context.Context, miners []minerActorInfo, sectorEvents chan<- *MinerSectorsEvent, sectorDeals chan<- *SectorDealEvent) error {
	start := time.Now()
	defer log.Debugw("Stored Miner PreCommit Info", "duration", time.Since(start).String())

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
			minerSectors, err := adt.AsArray(p.ctxStore, m.state.Sectors)
			if err != nil {
				return err
			}

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
				var sector miner.SectorOnChainInfo
				if found, err := minerSectors.Get(uint64(removed.Info.SectorNumber), &sector); err != nil {
					return err
				} else if !found {
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
	start := time.Now()
	defer log.Debugw("Stored Miner Sector Info", "duration", time.Since(start).String())

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
	start := time.Now()
	defer log.Debugw("Got Miner Partitions Differences", "duration", time.Since(start).String())

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

func (p *Processor) diffMinerPartitions(ctx context.Context, m minerActorInfo, events chan<- *MinerSectorsEvent) error {
	prevMiner, err := p.getMinerStateAt(ctx, m.common.addr, m.common.parentTsKey)
	if err != nil {
		return err
	}
	dlIdx := prevMiner.CurrentDeadline
	curMiner := m.state

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
}

func (p *Processor) getMinerStateAt(ctx context.Context, maddr address.Address, tskey types.TipSetKey) (miner.State, error) {
	prevActor, err := p.node.StateGetActor(ctx, maddr, tskey)
	if err != nil {
		return miner.State{}, err
	}

	var out miner.State
	// Get the miner state info
	astb, err := p.node.ChainReadObj(ctx, prevActor.Head)

	if err != nil {
		return miner.State{}, err
	}

	if err := out.UnmarshalCBOR(bytes.NewReader(astb)); err != nil {
		return miner.State{}, err
	}

	return out, nil
}

func (p *Processor) getMinerPreCommitChanges(ctx context.Context, m minerActorInfo) (*state.MinerPreCommitChanges, error) {
	pred := state.NewStatePredicates(p.node)
	changed, val, err := pred.OnMinerActorChange(m.common.addr, pred.OnMinerPreCommitChange())(ctx, m.common.parentTsKey, m.common.tsKey)

	if err != nil {
		return nil, xerrors.Errorf("Failed to diff miner precommit amt: %w", err)
	}

	if !changed {
		return nil, nil
	}

	out := val.(*state.MinerPreCommitChanges)
	return out, nil
}

func (p *Processor) getMinerSectorChanges(ctx context.Context, m minerActorInfo) (*state.MinerSectorChanges, error) {
	pred := state.NewStatePredicates(p.node)
	changed, val, err := pred.OnMinerActorChange(m.common.addr, pred.OnMinerSectorChange())(ctx, m.common.parentTsKey, m.common.tsKey)

	if err != nil {
		return nil, xerrors.Errorf("Failed to diff miner sectors amt: %w", err)
	}

	if !changed {
		return nil, nil
	}

	out := val.(*state.MinerSectorChanges)
	return out, nil
}

func (p *Processor) diffPartition(prevPart, curPart miner.Partition) (*PartitionStatus, error) {
	// all the sectors that were in previous but not in current
	allRemovedSectors, err := bitfield.SubtractBitField(prevPart.Sectors, curPart.Sectors)
	if err != nil {
		return nil, err
	}

	// list of sectors that were terminated before their expiration.
	terminatedEarlyArr, err := adt.AsArray(p.ctxStore, curPart.EarlyTerminated)
	if err != nil {
		return nil, err
	}

	expired := bitfield.New()
	var bf bitfield.BitField
	if err := terminatedEarlyArr.ForEach(&bf, func(i int64) error {
		// expired = all removals - termination
		expirations, err := bitfield.SubtractBitField(allRemovedSectors, bf)
		if err != nil {
			return err
		}
		// merge with expired sectors from other epochs
		expired, err = bitfield.MergeBitFields(expirations, expired)
		if err != nil {
			return nil
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// terminated = all removals - expired
	terminated, err := bitfield.SubtractBitField(allRemovedSectors, expired)
	if err != nil {
		return nil, err
	}

	// faults in current but not previous
	faults, err := bitfield.SubtractBitField(curPart.Recoveries, prevPart.Recoveries)
	if err != nil {
		return nil, err
	}

	// recoveries in current but not previous
	inRecovery, err := bitfield.SubtractBitField(curPart.Recoveries, prevPart.Recoveries)
	if err != nil {
		return nil, err
	}

	// all current good sectors
	newActiveSectors, err := curPart.ActiveSectors()
	if err != nil {
		return nil, err
	}

	// sectors that were previously fault and are now currently active are considered recovered.
	recovered, err := bitfield.IntersectBitField(prevPart.Faults, newActiveSectors)
	if err != nil {
		return nil, err
	}

	return &PartitionStatus{
		Terminated: terminated,
		Expired:    expired,
		Faulted:    faults,
		InRecovery: inRecovery,
		Recovered:  recovered,
	}, nil
}
