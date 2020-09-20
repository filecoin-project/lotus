package processor

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
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

type minerActorInfo struct {
	common actorInfo

	state miner.State

	// tracked by power actor
	rawPower big.Int
	qalPower big.Int
}

func (p *Processor) HandleMinerChanges(ctx context.Context, minerTips ActorTips) error {
	start := time.Now()
	defer log.Debugw("Handle Miner Changes", "duration", time.Since(start).String())

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

	var out []minerActorInfo
	grp, _ := errgroup.WithContext(ctx)
	var lk sync.Mutex

	for tipset, miners := range minerTips {
		tipset := tipset
		miners := miners

		grp.Go(func() error {
			// get the power actors claims map
			minersClaims, err := getPowerActorClaimsMap(ctx, p.node, tipset)
			if err != nil {
				//return nil, err
				return err
			}

			innerGrp, _ := errgroup.WithContext(ctx)
			// Get miner raw and quality power
			for _, act := range miners {
				act := act

				innerGrp.Go(func() error {
					var mi minerActorInfo
					mi.common = act

					var claim power.Claim
					// get miner claim from power actors claim map and store if found, else the miner had no claim at
					// this tipset
					found, err := minersClaims.Get(abi.AddrKey(act.addr), &claim)
					if err != nil {
						//return nil, err
						return err
					}
					if found {
						mi.qalPower = claim.QualityAdjPower
						mi.rawPower = claim.RawBytePower
					}

					// Get the miner state info
					astb, err := p.node.ChainReadObj(ctx, act.act.Head)
					if err != nil {
						log.Warnw("failed to find miner actor state", "address", act.addr, "error", err)
						//continue
						return nil
					}
					if err := mi.state.UnmarshalCBOR(bytes.NewReader(astb)); err != nil {
						//return nil, err
						return err
					}

					lk.Lock()
					out = append(out, mi)
					lk.Unlock()

					return nil
				})
			}

			return innerGrp.Wait()
		})
	}

	//return out, nil
	return out, grp.Wait()
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
		return p.storePreCommitDealInfo(ctx, dealEvents)
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

type minerInfo struct {
	minerActorInfo
	api.MinerInfo
}

func (p *Processor) fetchMinerInfos(ctx context.Context, miners []minerActorInfo) ([]minerInfo, error) {
	var lk sync.Mutex
	var minerInfos []minerInfo

	grp, _ := errgroup.WithContext(ctx)
	for _, m := range miners {
		m := m

		grp.Go(func() error {
			mi, err := p.node.StateMinerInfo(ctx, m.common.addr, m.common.tsKey)
			if err != nil {
				if !strings.Contains(err.Error(), types.ErrActorNotFound.Error()) {
					return err
				}
			} else {
				lk.Lock()
				minerInfos = append(minerInfos, minerInfo{m, mi})
				lk.Unlock()
			}
			return nil
		})
	}

	return minerInfos, grp.Wait()
}

func (p *Processor) storeMinersActorInfoState(ctx context.Context, miners []minerActorInfo) error {
	start := time.Now()
	defer func() {
		log.Debugw("Stored Miners Actor State", "duration", time.Since(start).String())
	}()

	mis, err := p.fetchMinerInfos(ctx, miners)
	if err != nil {
		return err
	}

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

	for _, mi := range mis {
		var pid string
		if mi.PeerId != nil {
			pid = mi.PeerId.String()
		}
		if _, err := stmt.Exec(
			mi.common.addr.String(),
			mi.Owner.String(),
			mi.Worker.String(),
			pid,
			mi.SectorSize.ShortString(),
		); err != nil {
			log.Errorw("failed to store miner state", "state", mi.state, "info", mi.state.Info, "error", err)
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
