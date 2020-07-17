package processor

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/events/state"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/api"
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
	
	precommit_deposits text not null,
	locked_funds text not null,
	next_deadline_process_faults bigint not null,
	constraint miner_info_pk
		primary key (miner_id)
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

create table if not exists miner_precommits
(
	miner_id text not null,
	sector_id bigint not null,
	
	precommit_deposit text not null,
	precommit_epoch text not null,
	constraint miner_precommits_pk
		primary key (miner_id, sector_id)
    
);

create table if not exists miner_sectors
(
	miner_id text not null,
	sector_id bigint not null,
	
	activation_epoch bigint not null,
	expiration_epoch bigint not null,
	termination_epoch bigint,
	
	deal_weight text not null,
	verified_deal_weight text not null,
	seal_cid text not null,
	seal_rand_epoch bigint not null,
	constraint miner_sectors_pk
		primary key (miner_id, sector_id)
);

/* used to tell when a miners sectors (proven-not-yet-expired) changed if the miner_sectors_cid's are different a new sector was added or removed (terminated/expired) */
create table if not exists miner_sectors_heads
(
	miner_id text not null,
	miner_sectors_cid text not null,
	
	state_root text not null,	
	
	constraint miner_sectors_heads_pk
		primary key (miner_id,miner_sectors_cid)
    
);


DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'miner_sector_event_type') THEN
        CREATE TYPE miner_sector_event_type AS ENUM
        (
			'PRECOMMIT', 'COMMIT', 'EXTENDED', 'EXPIRED', 'TERMINATED'
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

create materialized view if not exists miner_sectors_view as
select ms.miner_id, ms.sector_id, mp.precommit_epoch, ms.activation_epoch, ms.expiration_epoch, ms.termination_epoch, ms.deal_weight, ms.verified_deal_weight
from miner_sectors ms
left join miner_precommits mp on ms.sector_id = mp.sector_id and ms.miner_id = mp.miner_id

`); err != nil {
		return err
	}

	return tx.Commit()
}

type minerActorInfo struct {
	common actorInfo

	state miner.State

	// tracked by power actor
	rawPower big.Int
	qalPower big.Int
}

type sectorUpdate struct {
	terminationEpoch abi.ChainEpoch
	terminated       bool

	expirationEpoch abi.ChainEpoch

	sectorID abi.SectorNumber
	minerID  address.Address
}

func (p *Processor) HandleMinerChanges(ctx context.Context, minerTips ActorTips) error {
	minerChanges, err := p.processMiners(ctx, minerTips)
	if err != nil {
		log.Fatalw("Failed to process miner actors", "error", err)
	}

	if err := p.persistMiners(ctx, minerChanges); err != nil {
		log.Fatalw("Failed to persist miner actors", "error", err)
	}

	if err := p.updateMiners(ctx, minerChanges); err != nil {
		log.Fatalw("Failed to update miner actors", "error", err)
	}
	return nil
}

func (p *Processor) processMiners(ctx context.Context, minerTips map[types.TipSetKey][]actorInfo) ([]minerActorInfo, error) {
	start := time.Now()
	defer func() {
		log.Debugw("Processed Miners", "duration", time.Since(start).String())
	}()

	var out []minerActorInfo
	// TODO add parallel calls if this becomes slow
	for tipset, miners := range minerTips {
		// get the power actors claims map
		minersClaims, err := getPowerActorClaimsMap(ctx, p.node, tipset)
		if err != nil {
			return nil, err
		}

		// Get miner raw and quality power
		for _, act := range miners {
			var mi minerActorInfo
			mi.common = act

			var claim power.Claim
			// get miner claim from power actors claim map and store if found, else the miner had no claim at
			// this tipset
			found, err := minersClaims.Get(adt.AddrKey(act.addr), &claim)
			if err != nil {
				return nil, err
			}
			if found {
				mi.qalPower = claim.QualityAdjPower
				mi.rawPower = claim.RawBytePower
			}

			// Get the miner state info
			astb, err := p.node.ChainReadObj(ctx, act.act.Head)
			if err != nil {
				log.Warnw("failed to find miner actor state", "address", act.addr, "error", err)
				continue
			}
			if err := mi.state.UnmarshalCBOR(bytes.NewReader(astb)); err != nil {
				return nil, err
			}
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
		if err := p.storeMinersActorState(miners); err != nil {
			return err
		}
		return nil
	})

	grp.Go(func() error {
		if err := p.storeMinersPower(miners); err != nil {
			return err
		}
		return nil
	})

	grp.Go(func() error {
		if err := p.storeMinersSectorState(ctx, miners); err != nil {
			return err
		}
		return nil
	})

	grp.Go(func() error {
		if err := p.storeMinersSectorHeads(miners); err != nil {
			return err
		}
		return nil
	})

	grp.Go(func() error {
		if err := p.storeMinersPreCommitState(ctx, miners); err != nil {
			return err
		}
		return nil
	})

	return grp.Wait()
}

func (p *Processor) storeMinersActorState(miners []minerActorInfo) error {
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

	stmt, err := tx.Prepare(`copy mi (miner_id, owner_addr, worker_addr, peer_id, sector_size, precommit_deposits, locked_funds, next_deadline_process_faults) from STDIN`)
	if err != nil {
		return err
	}
	for _, m := range miners {
		var pid string
		if len(m.state.Info.PeerId) != 0 {
			peerid, err := peer.IDFromBytes(m.state.Info.PeerId)
			if err != nil {
				// this should "never happen", but if it does we should still store info about the miner.
				log.Warnw("failed to decode peerID", "peerID (bytes)", m.state.Info.PeerId, "miner", m.common.addr, "tipset", m.common.tsKey.String())
			} else {
				pid = peerid.String()
			}
		}
		if _, err := stmt.Exec(
			m.common.addr.String(),
			m.state.Info.Owner.String(),
			m.state.Info.Worker.String(),
			pid,
			m.state.Info.SectorSize.ShortString(),
			m.state.PreCommitDeposits.String(),
			m.state.LockedFunds.String(),
			m.state.NextDeadlineToProcessFaults,
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

func (p *Processor) storeMinersSectorState(ctx context.Context, miners []minerActorInfo) error {
	start := time.Now()
	defer func() {
		log.Debugw("Stored Miners Sector State", "duration", time.Since(start).String())
	}()

	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`create temp table ms (like miner_sectors excluding constraints) on commit drop;`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy ms (miner_id, sector_id, activation_epoch, expiration_epoch, deal_weight, verified_deal_weight, seal_cid, seal_rand_epoch) from STDIN`)
	if err != nil {
		return err
	}

	grp, ctx := errgroup.WithContext(ctx)
	for _, m := range miners {
		m := m
		grp.Go(func() error {
			sectors, err := p.node.StateMinerSectors(ctx, m.common.addr, nil, true, m.common.tsKey)
			if err != nil {
				log.Debugw("Failed to load sectors", "tipset", m.common.tsKey.String(), "miner", m.common.addr.String(), "error", err)
			}

			for _, sector := range sectors {
				if _, err := stmt.Exec(
					m.common.addr.String(),
					uint64(sector.ID),
					int64(sector.Info.ActivationEpoch),
					int64(sector.Info.Info.Expiration),
					sector.Info.DealWeight.String(),
					sector.Info.VerifiedDealWeight.String(),
					sector.Info.Info.SealedCID.String(),
					int64(sector.Info.Info.SealRandEpoch),
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
		return err
	}

	if _, err := tx.Exec(`insert into miner_sectors select * from ms on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	return tx.Commit()
}

func (p *Processor) storeMinersSectorHeads(miners []minerActorInfo) error {
	start := time.Now()
	defer func() {
		log.Debugw("Stored Miners Sector Heads", "duration", time.Since(start).String())
	}()

	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`create temp table msh (like miner_sectors_heads excluding constraints) on commit drop;`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy msh (miner_id, miner_sectors_cid, state_root) from STDIN`)
	if err != nil {
		return err
	}

	for _, m := range miners {
		if _, err := stmt.Exec(
			m.common.addr.String(),
			m.state.Sectors.String(),
			m.common.stateroot.String(),
		); err != nil {
			log.Errorw("failed to store miners sectors head", "state", m.state, "info", m.state.Info, "error", err)
			return err
		}

	}
	if err := stmt.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`insert into miner_sectors_heads select * from msh on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	return tx.Commit()
}

func (p *Processor) storeMinersPreCommitState(ctx context.Context, miners []minerActorInfo) error {
	start := time.Now()
	defer func() {
		log.Infow("Stored Miners Precommit State", "duration", time.Since(start).String())
	}()

	precommitTx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := precommitTx.Exec(`create temp table mp (like miner_precommits excluding constraints) on commit drop;`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	precommitStmt, err := precommitTx.Prepare(`copy mp (miner_id, sector_id, precommit_deposit, precommit_epoch) from STDIN`)
	if err != nil {
		return err
	}

	for _, m := range miners {
		m := m
		pcMap, err := adt.AsMap(cw_util.NewAPIIpldStore(ctx, p.node), m.state.PreCommittedSectors)
		if err != nil {
			return err
		}
		precommit := new(miner.SectorPreCommitOnChainInfo)
		if err := pcMap.ForEach(precommit, func(key string) error {
			if _, err := precommitStmt.Exec(
				m.common.addr.String(),
				precommit.Info.SectorNumber,
				precommit.PreCommitDeposit.String(),
				precommit.PreCommitEpoch,
			); err != nil {
				return err
			}

			return nil
		}); err != nil {
			return err
		}
	}
	if err := precommitStmt.Close(); err != nil {
		return err
	}
	if _, err := precommitTx.Exec(`insert into miner_precommits select * from mp on conflict do nothing`); err != nil {
		return err
	}

	return precommitTx.Commit()
}

func (p *Processor) updateMiners(ctx context.Context, miners []minerActorInfo) error {
	// TODO when/if there is more than one update operation here use an errgroup as is done in persistMiners
	if err := p.updateMinersSectors(ctx, miners); err != nil {
		return err
	}

	if err := p.updateMinersPrecommits(ctx, miners); err != nil {
		return err
	}
	return nil
}

func (p *Processor) updateMinersPrecommits(ctx context.Context, miners []minerActorInfo) error {
	start := time.Now()
	defer func() {
		log.Infow("Updated Miner Precommits", "duration", time.Since(start).String())
	}()

	pred := state.NewStatePredicates(p.node)

	eventTx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := eventTx.Exec(`create temp table mse (like miner_sector_events excluding constraints) on commit drop;`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	eventStmt, err := eventTx.Prepare(`copy mse (sector_id, event, miner_id, state_root) from STDIN `)
	if err != nil {
		return err
	}

	for _, m := range miners {
		pcDiffFn := pred.OnMinerActorChange(m.common.addr, pred.OnMinerPreCommitChange())
		changed, val, err := pcDiffFn(ctx, m.common.parentTsKey, m.common.tsKey)
		if err != nil {
			if strings.Contains(err.Error(), "address not found") {
				continue
			}
			log.Errorw("error getting miner precommit diff", "miner", m.common.addr, "error", err)
			return err
		}
		if !changed {
			continue
		}
		changes, ok := val.(*state.MinerPreCommitChanges)
		if !ok {
			log.Fatal("Developer Error")
		}
		for _, added := range changes.Added {
			if _, err := eventStmt.Exec(added.Info.SectorNumber, "PRECOMMIT", m.common.addr.String(), m.common.stateroot.String()); err != nil {
				return err
			}
		}
	}

	if err := eventStmt.Close(); err != nil {
		return err
	}

	if _, err := eventTx.Exec(`insert into miner_sector_events select * from mse on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	return eventTx.Commit()
}

func (p *Processor) updateMinersSectors(ctx context.Context, miners []minerActorInfo) error {
	log.Debugw("Updating Miners Sectors", "#miners", len(miners))
	start := time.Now()
	defer func() {
		log.Debugw("Updated Miners Sectors", "duration", time.Since(start).String())
	}()

	pred := state.NewStatePredicates(p.node)

	eventTx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := eventTx.Exec(`create temp table mse (like miner_sector_events excluding constraints) on commit drop;`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	eventStmt, err := eventTx.Prepare(`copy mse (sector_id, event, miner_id, state_root) from STDIN `)
	if err != nil {
		return err
	}

	var updateWg sync.WaitGroup
	updateWg.Add(1)
	sectorUpdatesCh := make(chan sectorUpdate)
	var sectorUpdates []sectorUpdate
	go func() {
		for u := range sectorUpdatesCh {
			sectorUpdates = append(sectorUpdates, u)
		}
		updateWg.Done()
	}()

	minerGrp, ctx := errgroup.WithContext(ctx)
	complete := 0
	for _, m := range miners {
		m := m
		if m.common.tsKey == p.genesisTs.Key() {
			genSectors, err := p.node.StateMinerSectors(ctx, m.common.addr, nil, true, p.genesisTs.Key())
			if err != nil {
				return err
			}
			for _, sector := range genSectors {
				if _, err := eventStmt.Exec(sector.ID, "COMMIT", m.common.addr.String(), m.common.stateroot.String()); err != nil {
					return err
				}
			}
			complete++
			continue
		}
		minerGrp.Go(func() error {
			// special case genesis miners
			sectorDiffFn := pred.OnMinerActorChange(m.common.addr, pred.OnMinerSectorChange())
			changed, val, err := sectorDiffFn(ctx, m.common.parentTsKey, m.common.tsKey)
			if err != nil {
				if strings.Contains(err.Error(), "address not found") {
					return nil
				}
				log.Errorw("error getting miner sector diff", "miner", m.common.addr, "error", err)
				return err
			}
			if !changed {
				complete++
				return nil
			}
			changes, ok := val.(*state.MinerSectorChanges)
			if !ok {
				log.Fatalw("Developer Error")
			}
			log.Debugw("sector changes for miner", "miner", m.common.addr.String(), "Added", len(changes.Added), "Extended", len(changes.Extended), "Removed", len(changes.Removed), "oldState", m.common.parentTsKey, "newState", m.common.tsKey)

			for _, extended := range changes.Extended {
				if _, err := eventStmt.Exec(extended.To.Info.SectorNumber, "EXTENDED", m.common.addr.String(), m.common.stateroot.String()); err != nil {
					return err
				}
				sectorUpdatesCh <- sectorUpdate{
					terminationEpoch: 0,
					terminated:       false,
					expirationEpoch:  extended.To.Info.Expiration,
					sectorID:         extended.From.Info.SectorNumber,
					minerID:          m.common.addr,
				}

				log.Debugw("sector extended", "miner", m.common.addr.String(), "sector", extended.To.Info.SectorNumber, "old", extended.To.Info.Expiration, "new", extended.From.Info.Expiration)
			}
			curTs, err := p.node.ChainGetTipSet(ctx, m.common.tsKey)
			if err != nil {
				return err
			}

			for _, removed := range changes.Removed {
				log.Debugw("removed", "miner", m.common.addr)
				// decide if they were terminated or extended
				if removed.Info.Expiration > curTs.Height() {
					if _, err := eventStmt.Exec(removed.Info.SectorNumber, "TERMINATED", m.common.addr.String(), m.common.stateroot.String()); err != nil {
						return err
					}
					log.Debugw("sector terminated", "miner", m.common.addr.String(), "sector", removed.Info.SectorNumber, "old", "sectorExpiration", removed.Info.Expiration, "terminationEpoch", curTs.Height())
					sectorUpdatesCh <- sectorUpdate{
						terminationEpoch: curTs.Height(),
						terminated:       true,
						expirationEpoch:  removed.Info.Expiration,
						sectorID:         removed.Info.SectorNumber,
						minerID:          m.common.addr,
					}

				}
				if _, err := eventStmt.Exec(removed.Info.SectorNumber, "EXPIRED", m.common.addr.String(), m.common.stateroot.String()); err != nil {
					return err
				}
				log.Debugw("sector removed", "miner", m.common.addr.String(), "sector", removed.Info.SectorNumber, "old", "sectorExpiration", removed.Info.Expiration, "currEpoch", curTs.Height())
			}

			for _, added := range changes.Added {
				if _, err := eventStmt.Exec(added.Info.SectorNumber, "COMMIT", m.common.addr.String(), m.common.stateroot.String()); err != nil {
					return err
				}
			}
			complete++
			log.Debugw("Update Done", "complete", complete, "added", len(changes.Added), "removed", len(changes.Removed), "modified", len(changes.Extended))
			return nil
		})
	}
	if err := minerGrp.Wait(); err != nil {
		return err
	}
	close(sectorUpdatesCh)
	// wait for the update channel to be drained
	updateWg.Wait()

	if err := eventStmt.Close(); err != nil {
		return err
	}

	if _, err := eventTx.Exec(`insert into miner_sector_events select * from mse on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	if err := eventTx.Commit(); err != nil {
		return err
	}

	updateTx, err := p.db.Begin()
	if err != nil {
		return err
	}

	updateStmt, err := updateTx.Prepare(`UPDATE miner_sectors SET termination_epoch=$1, expiration_epoch=$2 WHERE miner_id=$3 AND sector_id=$4`)
	if err != nil {
		return err
	}

	for _, update := range sectorUpdates {
		if update.terminated {
			if _, err := updateStmt.Exec(update.terminationEpoch, update.expirationEpoch, update.minerID.String(), update.sectorID); err != nil {
				return err
			}
		} else {
			if _, err := updateStmt.Exec(nil, update.expirationEpoch, update.minerID.String(), update.sectorID); err != nil {
				return err
			}
		}
	}

	if err := updateStmt.Close(); err != nil {
		return err
	}

	return updateTx.Commit()
}

// load the power actor state clam as an adt.Map at the tipset `ts`.
func getPowerActorClaimsMap(ctx context.Context, api api.FullNode, ts types.TipSetKey) (*adt.Map, error) {
	powerActor, err := api.StateGetActor(ctx, builtin.StoragePowerActorAddr, ts)
	if err != nil {
		return nil, err
	}

	powerRaw, err := api.ChainReadObj(ctx, powerActor.Head)
	if err != nil {
		return nil, err
	}

	var powerActorState power.State
	if err := powerActorState.UnmarshalCBOR(bytes.NewReader(powerRaw)); err != nil {
		return nil, fmt.Errorf("failed to unmarshal power actor state: %w", err)
	}

	s := cw_util.NewAPIIpldStore(ctx, api)
	return adt.AsMap(s, powerActorState.Claims)
}
