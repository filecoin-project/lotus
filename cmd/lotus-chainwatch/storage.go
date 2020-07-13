package main

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/ipfs/go-cid"
	_ "github.com/lib/pq"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/events/state"
	"github.com/filecoin-project/lotus/chain/types"
)

type storage struct {
	db *sql.DB

	headerLk sync.Mutex

	genesisTs *types.TipSet
}

func openStorage(dbSource string) (*storage, error) {
	db, err := sql.Open("postgres", dbSource)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1350)

	st := &storage{db: db}

	return st, st.setup()
}

func (st *storage) setup() error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
create table if not exists block_cids
(
	cid text not null
		constraint block_cids_pk
			primary key
);

create unique index if not exists block_cids_cid_uindex
	on block_cids (cid);

create table if not exists blocks_synced
(
	cid text not null
		constraint blocks_synced_pk
			primary key
	    constraint blocks_block_cids_cid_fk
			references block_cids (cid),
	add_ts int not null
);

create unique index if not exists blocks_synced_cid_uindex
	on blocks_synced (cid);

create table if not exists block_parents
(
	block text not null
	    constraint blocks_block_cids_cid_fk
			references block_cids (cid),
	parent text not null
);

create unique index if not exists block_parents_block_parent_uindex
	on block_parents (block, parent);

create table if not exists drand_entries
(
    round bigint not null
    	constraint drand_entries_pk
			primary key,
	data bytea not null
);
create unique index if not exists drand_entries_round_uindex
	on drand_entries (round);

create table if not exists block_drand_entries
(
    round bigint not null
    	constraint block_drand_entries_drand_entries_round_fk
			references drand_entries (round),
	block text not null
	    constraint blocks_block_cids_cid_fk
			references block_cids (cid)
);
create unique index if not exists block_drand_entries_round_uindex
	on block_drand_entries (round, block);

create table if not exists blocks
(
	cid text not null
		constraint blocks_pk
			primary key
	    constraint blocks_block_cids_cid_fk
			references block_cids (cid),
	parentWeight numeric not null,
	parentStateRoot text not null,
	height bigint not null,
	miner text not null,
	timestamp bigint not null,
	ticket bytea not null,
	eprof bytea,
	forksig bigint not null
);

create unique index if not exists block_cid_uindex
	on blocks (cid);

create materialized view if not exists state_heights
    as select distinct height, parentstateroot from blocks;

create index if not exists state_heights_index
	on state_heights (height);

create index if not exists state_heights_height_index
	on state_heights (parentstateroot);

create table if not exists id_address_map
(
	id text not null,
	address text not null,
	constraint id_address_map_pk
		primary key (id, address)
);

create unique index if not exists id_address_map_id_uindex
	on id_address_map (id);

create unique index if not exists id_address_map_address_uindex
	on id_address_map (address);

create table if not exists actors
  (
	id text not null
		constraint id_address_map_actors_id_fk
			references id_address_map (id),
	code text not null,
	head text not null,
	nonce int not null,
	balance text not null,
	stateroot text
  );
  
create index if not exists actors_id_index
	on actors (id);

create index if not exists id_address_map_address_index
	on id_address_map (address);

create index if not exists id_address_map_id_index
	on id_address_map (id);

create or replace function actor_tips(epoch bigint)
    returns table (id text,
                    code text,
                    head text,
                    nonce int,
                    balance text,
                    stateroot text,
                    height bigint,
                    parentstateroot text) as
$body$
    select distinct on (id) * from actors
        inner join state_heights sh on sh.parentstateroot = stateroot
        where height < $1
		order by id, height desc;
$body$ language sql;

create table if not exists actor_states
(
	head text not null,
	code text not null,
	state json not null
);

create unique index if not exists actor_states_head_code_uindex
	on actor_states (head, code);

create index if not exists actor_states_head_index
	on actor_states (head);

create index if not exists actor_states_code_head_index
	on actor_states (head, code);

create table if not exists messages
(
	cid text not null
		constraint messages_pk
			primary key,
	"from" text not null,
	"to" text not null,
	nonce bigint not null,
	value text not null,
	gasprice bigint not null,
	gaslimit bigint not null,
	method bigint,
	params bytea
);

create unique index if not exists messages_cid_uindex
	on messages (cid);

create index if not exists messages_from_index
	on messages ("from");

create index if not exists messages_to_index
	on messages ("to");

create table if not exists block_messages
(
	block text not null
	    constraint blocks_block_cids_cid_fk
			references block_cids (cid),
	message text not null,
	constraint block_messages_pk
		primary key (block, message)
);

create table if not exists mpool_messages
(
	msg text not null
		constraint mpool_messages_pk
			primary key
		constraint mpool_messages_messages_cid_fk
			references messages,
	add_ts int not null
);

create unique index if not exists mpool_messages_msg_uindex
	on mpool_messages (msg);

create table if not exists receipts
(
	msg text not null,
	state text not null,
	idx int not null,
	exit int not null,
	gas_used int not null,
	return bytea,
	constraint receipts_pk
		primary key (msg, state)
);

create index if not exists receipts_msg_state_index
	on receipts (msg, state);
	
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

create index if not exists miner_sectors_miner_sectorid_index
	on miner_sectors (miner_id, sector_id);

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
* captures chain-specific power state for any given stateroot
*/
create table if not exists chain_power
(
	state_root text not null
		constraint chain_power_pk
			primary key,
	baseline_power text not null
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

/* used to tell when a miners sectors (proven-not-yet-expired) changed if the miner_sectors_cid's are different a new sector was added or removed (terminated/expired) */
create table if not exists miner_sectors_heads
(
	miner_id text not null,
	miner_sectors_cid text not null,
	
	state_root text not null,	
	
	constraint miner_sectors_heads_pk
		primary key (miner_id,miner_sectors_cid)
    
);

create type miner_sector_event_type as enum ('ADDED', 'EXTENDED', 'EXPIRED', 'TERMINATED');

create table if not exists miner_sector_events
(
    miner_id text not null,
    sector_id bigint not null,
    state_root text not null,
    event miner_sector_event_type not null,
    
	constraint miner_sector_events_pk
		primary key (sector_id, event, miner_id, state_root)
)

/*
create or replace function miner_tips(epoch bigint)
    returns table (head text,
                   addr text,
                   stateroot text,
                   sectorset text,
                   setsize decimal,
                   provingset text,
                   provingsize decimal,
                   owner text,
                   worker text,
                   peerid text,
                   sectorsize bigint,
                   power decimal,
                   active bool,
                   ppe bigint,
                   slashed_at bigint,
                   height bigint,
                   parentstateroot text) as
    $body$
        select distinct on (addr) * from miner_heads
            inner join state_heights sh on sh.parentstateroot = stateroot
            where height < $1
            order by addr, height desc;
    $body$ language sql;

create table if not exists deals
(
	id int not null,
	pieceRef text not null,
	pieceSize bigint not null,
	client text not null,
	provider text not null,
	start decimal not null,
	end decimal not null,
	epochPrice decimal not null,
	collateral decimal not null,
	constraint deals_pk
		primary key (id)
);

create index if not exists deals_client_index
	on deals (client);

create unique index if not exists deals_id_uindex
	on deals (id);

create index if not exists deals_pieceRef_index
	on deals (pieceRef);

create index if not exists deals_provider_index
	on deals (provider);

create table if not exists deal_activations
(
	deal bigint not null
		constraint deal_activations_deals_id_fk
			references deals,
	activation_epoch bigint not null,
	constraint deal_activations_pk
		primary key (deal)
);

create index if not exists deal_activations_activation_epoch_index
	on deal_activations (activation_epoch);

create unique index if not exists deal_activations_deal_uindex
	on deal_activations (deal);
*/

`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (st *storage) hasList() map[cid.Cid]struct{} {
	rws, err := st.db.Query(`select cid FROM blocks_synced`)
	if err != nil {
		log.Error(err)
		return map[cid.Cid]struct{}{}
	}
	out := map[cid.Cid]struct{}{}

	for rws.Next() {
		var c string
		if err := rws.Scan(&c); err != nil {
			log.Error(err)
			continue
		}

		ci, err := cid.Parse(c)
		if err != nil {
			log.Error(err)
			continue
		}

		out[ci] = struct{}{}
	}

	return out
}

func (st *storage) storeActors(actors map[address.Address]map[types.Actor]actorInfo) error {
	// Basic
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`
		create temp table a (like actors excluding constraints) on commit drop;
	`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy a (id, code, head, nonce, balance, stateroot) from stdin `)
	if err != nil {
		return err
	}

	for addr, acts := range actors {
		for act, st := range acts {
			if _, err := stmt.Exec(addr.String(), act.Code.String(), act.Head.String(), act.Nonce, act.Balance.String(), st.stateroot.String()); err != nil {
				return err
			}
		}
	}

	if err := stmt.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`insert into actors select * from a on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// States
	tx, err = st.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`
		create temp table a (like actor_states excluding constraints) on commit drop;
	`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	stmt, err = tx.Prepare(`copy a (head, code, state) from stdin `)
	if err != nil {
		return err
	}

	for _, acts := range actors {
		for act, st := range acts {
			if _, err := stmt.Exec(act.Head.String(), act.Code.String(), st.state); err != nil {
				return err
			}
		}
	}

	if err := stmt.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`insert into actor_states select * from a on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

// storeChainPower captures reward actor state as it relates to power captured on-chain
func (st *storage) storeChainPower(rewardTips map[types.TipSetKey]*rewardStateInfo) error {
	tx, err := st.db.Begin()
	if err != nil {
		return xerrors.Errorf("begin chain_power tx: %w", err)
	}

	if _, err := tx.Exec(`create temp table cp (like chain_power excluding constraints) on commit drop`); err != nil {
		return xerrors.Errorf("prep chain_power temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy cp (state_root, baseline_power) from STDIN`)
	if err != nil {
		return xerrors.Errorf("prepare tmp chain_power: %w", err)
	}

	for _, rewardState := range rewardTips {
		if _, err := stmt.Exec(
			rewardState.stateroot.String(),
			rewardState.baselinePower.String(),
		); err != nil {
			log.Errorw("failed to store chain power", "state_root", rewardState.stateroot, "error", err)
		}
	}

	if err := stmt.Close(); err != nil {
		return xerrors.Errorf("close prepared chain_power: %w", err)
	}

	if _, err := tx.Exec(`insert into chain_power select * from cp on conflict do nothing`); err != nil {
		return xerrors.Errorf("insert chain_power from tmp: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("commit chain_power tx: %w", err)
	}

	return nil
}

type storeSectorsAPI interface {
	StateMinerSectors(context.Context, address.Address, *abi.BitField, bool, types.TipSetKey) ([]*api.ChainSectorInfo, error)
}

func (st *storage) storeSectors(minerTips map[types.TipSetKey][]*minerStateInfo, sectorApi storeSectorsAPI) error {
	tx, err := st.db.Begin()
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

	for tipset, miners := range minerTips {
		for _, miner := range miners {
			sectors, err := sectorApi.StateMinerSectors(context.TODO(), miner.addr, nil, true, tipset)
			if err != nil {
				log.Debugw("Failed to load sectors", "tipset", tipset.String(), "miner", miner.addr.String(), "error", err)
			}

			for _, sector := range sectors {
				if _, err := stmt.Exec(
					miner.addr.String(),
					uint64(sector.ID),
					int64(sector.Info.Activation),
					int64(sector.Info.Expiration),
					sector.Info.DealWeight.String(),
					sector.Info.VerifiedDealWeight.String(),
					sector.Info.SealedCID.String(),
					0, // TODO: Not there now?
				); err != nil {
					return err
				}
			}
		}
	}

	if err := stmt.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`insert into miner_sectors select * from ms on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	return tx.Commit()
}

func (st *storage) storeMiners(minerTips map[types.TipSetKey][]*minerStateInfo) error {
	tx, err := st.db.Begin()
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
	for ts, miners := range minerTips {
		for _, miner := range miners {
			var pid string
			if len(miner.info.PeerId) != 0 {
				peerid, err := peer.IDFromBytes(miner.info.PeerId)
				if err != nil {
					// this should "never happen", but if it does we should still store info about the miner.
					log.Warnw("failed to decode peerID", "peerID (bytes)", miner.info.PeerId, "miner", miner.addr, "tipset", ts.String())
				} else {
					pid = peerid.String()
				}
			}
			if _, err := stmt.Exec(
				miner.addr.String(),
				miner.info.Owner.String(),
				miner.info.Worker.String(),
				pid,
				miner.info.SectorSize.ShortString(),
				miner.state.PreCommitDeposits.String(),
				miner.state.LockedFunds.String(),
				miner.state.NextDeadlineToProcessFaults,
			); err != nil {
				log.Errorw("failed to store miner state", "state", miner.state, "info", miner.info, "error", err)
				return err
			}

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

// storeMinerPower captures miner actor state as it relates to power per miner captured on-chain
func (st *storage) storeMinerPower(minerTips map[types.TipSetKey][]*minerStateInfo) error {
	tx, err := st.db.Begin()
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

	for _, miners := range minerTips {
		for _, minerInfo := range miners {
			if _, err := stmt.Exec(
				minerInfo.addr.String(),
				minerInfo.stateroot.String(),
				minerInfo.rawPower.String(),
				minerInfo.qalPower.String(),
			); err != nil {
				log.Errorw("failed to store miner power", "miner", minerInfo.addr, "stateroot", minerInfo.stateroot, "error", err)
			}
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

func (st *storage) storeMinerSectorsHeads(minerTips map[types.TipSetKey][]*minerStateInfo, api api.FullNode) error {
	tx, err := st.db.Begin()
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

	for _, miners := range minerTips {
		for _, miner := range miners {
			if _, err := stmt.Exec(
				miner.addr.String(),
				miner.state.Sectors.String(),
				miner.stateroot.String(),
			); err != nil {
				log.Errorw("failed to store miners sectors head", "state", miner.state, "info", miner.info, "error", err)
				return err
			}

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

type sectorUpdate struct {
	terminationEpoch abi.ChainEpoch
	terminated       bool

	expirationEpoch abi.ChainEpoch

	sectorID abi.SectorNumber
	minerID  address.Address
}

func (st *storage) updateMinerSectors(minerTips map[types.TipSetKey][]*minerStateInfo, api api.FullNode) error {
	log.Debugw("updating miners constant sector table", "#tipsets", len(minerTips))
	pred := state.NewStatePredicates(api)

	eventTx, err := st.db.Begin()
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

	var sectorUpdates []sectorUpdate
	// TODO consider performing the miner sector diffing in parallel and performing the database update after.
	for _, miners := range minerTips {
		for _, miner := range miners {
			// special case genesis miners
			if miner.tsKey == st.genesisTs.Key() {
				sectors, err := api.StateMinerSectors(context.TODO(), miner.addr, nil, true, miner.tsKey)
				if err != nil {
					log.Debugw("failed to get miner info for genesis", "miner", miner.addr.String())
					continue
				}

				for _, sector := range sectors {
					if _, err := eventStmt.Exec(sector.Info.SectorNumber, "ADDED", miner.addr.String(), miner.stateroot.String()); err != nil {
						return err
					}
				}
			} else {
				sectorDiffFn := pred.OnMinerActorChange(miner.addr, pred.OnMinerSectorChange())
				changed, val, err := sectorDiffFn(context.TODO(), miner.parentTsKey, miner.tsKey)
				if err != nil {
					log.Debugw("error getting miner sector diff", "miner", miner.addr, "error", err)
					continue
				}
				if !changed {
					continue
				}
				changes := val.(*state.MinerSectorChanges)
				log.Debugw("sector changes for miner", "miner", miner.addr.String(), "Added", len(changes.Added), "Extended", len(changes.Extended), "Removed", len(changes.Removed), "oldState", miner.parentTsKey, "newState", miner.tsKey)

				for _, extended := range changes.Extended {
					if _, err := eventStmt.Exec(extended.To.SectorNumber, "EXTENDED", miner.addr.String(), miner.stateroot.String()); err != nil {
						return err
					}
					sectorUpdates = append(sectorUpdates, sectorUpdate{
						terminationEpoch: 0,
						terminated:       false,
						expirationEpoch:  extended.To.Expiration,
						sectorID:         extended.To.SectorNumber,
						minerID:          miner.addr,
					})
					log.Debugw("sector extended", "miner", miner.addr.String(), "sector", extended.To.SectorNumber, "old", extended.To.Expiration, "new", extended.From.Expiration)
				}
				curTs, err := api.ChainGetTipSet(context.TODO(), miner.tsKey)
				if err != nil {
					return err
				}

				for _, removed := range changes.Removed {
					// decide if they were terminated or extended
					if removed.Expiration > curTs.Height() {
						if _, err := eventStmt.Exec(removed.SectorNumber, "TERMINATED", miner.addr.String(), miner.stateroot.String()); err != nil {
							return err
						}
						log.Debugw("sector terminated", "miner", miner.addr.String(), "sector", removed.SectorNumber, "old", "sectorExpiration", removed.Expiration, "terminationEpoch", curTs.Height())
						sectorUpdates = append(sectorUpdates, sectorUpdate{
							terminationEpoch: curTs.Height(),
							terminated:       true,
							expirationEpoch:  removed.Expiration,
							sectorID:         removed.SectorNumber,
							minerID:          miner.addr,
						})
					}
					if _, err := eventStmt.Exec(removed.SectorNumber, "EXPIRED", miner.addr.String(), miner.stateroot.String()); err != nil {
						return err
					}
					log.Debugw("sector removed", "miner", miner.addr.String(), "sector", removed.SectorNumber, "old", "sectorExpiration", removed.Expiration, "currEpoch", curTs.Height())
				}

				for _, added := range changes.Added {
					if _, err := eventStmt.Exec(miner.addr.String(), added.SectorNumber, miner.stateroot.String(), "ADDED"); err != nil {
						return err
					}
				}
			}
		}
	}
	if err := eventStmt.Close(); err != nil {
		return err
	}

	if _, err := eventTx.Exec(`insert into miner_sector_events select * from mse on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	if err := eventTx.Commit(); err != nil {
		return err
	}

	updateTx, err := st.db.Begin()
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

func (st *storage) storeHeaders(bhs map[cid.Cid]*types.BlockHeader, sync bool) error {
	st.headerLk.Lock()
	defer st.headerLk.Unlock()

	tx, err := st.db.Begin()
	if err != nil {
		return xerrors.Errorf("begin: %w", err)
	}

	if _, err := tx.Exec(`

create temp table bc (like block_cids excluding constraints) on commit drop;
create temp table de (like drand_entries excluding constraints) on commit drop;
create temp table bde (like block_drand_entries excluding constraints) on commit drop;
create temp table tbp (like block_parents excluding constraints) on commit drop;
create temp table bs (like blocks_synced excluding constraints) on commit drop;
create temp table b (like blocks excluding constraints) on commit drop;


`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	{
		stmt, err := tx.Prepare(`copy bc (cid) from STDIN`)
		if err != nil {
			return err
		}

		for _, bh := range bhs {
			if _, err := stmt.Exec(bh.Cid().String()); err != nil {
				log.Error(err)
			}
		}

		if err := stmt.Close(); err != nil {
			return err
		}

		if _, err := tx.Exec(`insert into block_cids select * from bc on conflict do nothing `); err != nil {
			return xerrors.Errorf("drand entries put: %w", err)
		}
	}

	{
		stmt, err := tx.Prepare(`copy de (round, data) from STDIN`)
		if err != nil {
			return err
		}

		for _, bh := range bhs {
			for _, ent := range bh.BeaconEntries {
				if _, err := stmt.Exec(ent.Round, ent.Data); err != nil {
					log.Error(err)
				}
			}
		}

		if err := stmt.Close(); err != nil {
			return err
		}

		if _, err := tx.Exec(`insert into drand_entries select * from de on conflict do nothing `); err != nil {
			return xerrors.Errorf("drand entries put: %w", err)
		}
	}

	{
		stmt, err := tx.Prepare(`copy bde (round, block) from STDIN`)
		if err != nil {
			return err
		}

		for _, bh := range bhs {
			for _, ent := range bh.BeaconEntries {
				if _, err := stmt.Exec(ent.Round, bh.Cid().String()); err != nil {
					log.Error(err)
				}
			}
		}

		if err := stmt.Close(); err != nil {
			return err
		}

		if _, err := tx.Exec(`insert into block_drand_entries select * from bde on conflict do nothing `); err != nil {
			return xerrors.Errorf("block drand entries put: %w", err)
		}
	}

	{
		stmt, err := tx.Prepare(`copy tbp (block, parent) from STDIN`)
		if err != nil {
			return err
		}

		for _, bh := range bhs {
			for _, parent := range bh.Parents {
				if _, err := stmt.Exec(bh.Cid().String(), parent.String()); err != nil {
					log.Error(err)
				}
			}
		}

		if err := stmt.Close(); err != nil {
			return err
		}

		if _, err := tx.Exec(`insert into block_parents select * from tbp on conflict do nothing `); err != nil {
			return xerrors.Errorf("parent put: %w", err)
		}
	}

	if sync {
		now := time.Now().Unix()

		stmt, err := tx.Prepare(`copy bs (cid, add_ts) from stdin `)
		if err != nil {
			return err
		}

		for _, bh := range bhs {
			if _, err := stmt.Exec(bh.Cid().String(), now); err != nil {
				log.Error(err)
			}
		}

		if err := stmt.Close(); err != nil {
			return err
		}

		if _, err := tx.Exec(`insert into blocks_synced select * from bs on conflict do nothing `); err != nil {
			return xerrors.Errorf("syncd put: %w", err)
		}
	}

	stmt2, err := tx.Prepare(`copy b (cid, parentWeight, parentStateRoot, height, miner, "timestamp", ticket, eprof, forksig) from stdin`)
	if err != nil {
		return err
	}

	for _, bh := range bhs {
		var eprof interface{}
		if bh.ElectionProof != nil {
			eprof = bh.ElectionProof.VRFProof
		}

		if bh.Ticket == nil {
			log.Warnf("got a block with nil ticket")

			bh.Ticket = &types.Ticket{
				VRFProof: []byte{},
			}
		}

		if _, err := stmt2.Exec(
			bh.Cid().String(),
			bh.ParentWeight.String(),
			bh.ParentStateRoot.String(),
			bh.Height,
			bh.Miner.String(),
			bh.Timestamp,
			bh.Ticket.VRFProof,
			eprof,
			bh.ForkSignaling); err != nil {
			log.Error(err)
		}
	}

	if err := stmt2.Close(); err != nil {
		return xerrors.Errorf("s2 close: %w", err)
	}

	if _, err := tx.Exec(`insert into blocks select * from b on conflict do nothing `); err != nil {
		return xerrors.Errorf("blk put: %w", err)
	}

	return tx.Commit()
}

func (st *storage) storeMessages(msgs map[cid.Cid]*types.Message) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`

create temp table msgs (like messages excluding constraints) on commit drop;


`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy msgs (cid, "from", "to", nonce, "value", gasprice, gaslimit, method, params) from stdin `)
	if err != nil {
		return err
	}

	for c, m := range msgs {
		if _, err := stmt.Exec(
			c.String(),
			m.From.String(),
			m.To.String(),
			m.Nonce,
			m.Value.String(),
			m.GasPrice.String(),
			m.GasLimit,
			m.Method,
			m.Params,
		); err != nil {
			return err
		}
	}
	if err := stmt.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`insert into messages select * from msgs on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	return tx.Commit()
}

func (st *storage) storeReceipts(recs map[mrec]*types.MessageReceipt) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`

create temp table recs (like receipts excluding constraints) on commit drop;


`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy recs (msg, state, idx, exit, gas_used, return) from stdin `)
	if err != nil {
		return err
	}

	for c, m := range recs {
		if _, err := stmt.Exec(
			c.msg.String(),
			c.state.String(),
			c.idx,
			m.ExitCode,
			m.GasUsed,
			m.Return,
		); err != nil {
			return err
		}
	}
	if err := stmt.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`insert into receipts select * from recs on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	return tx.Commit()
}

func (st *storage) storeAddressMap(addrs map[address.Address]address.Address) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`

create temp table iam (like id_address_map excluding constraints) on commit drop;


`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy iam (id, address) from STDIN `)
	if err != nil {
		return err
	}

	for a, i := range addrs {
		if i == address.Undef {
			continue
		}
		if _, err := stmt.Exec(
			i.String(),
			a.String(),
		); err != nil {
			return err
		}
	}
	if err := stmt.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`insert into id_address_map select * from iam on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	return tx.Commit()
}

func (st *storage) storeMsgInclusions(incls map[cid.Cid][]cid.Cid) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`

create temp table mi (like block_messages excluding constraints) on commit drop;


`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy mi (block, message) from STDIN `)
	if err != nil {
		return err
	}

	for b, msgs := range incls {
		for _, msg := range msgs {
			if _, err := stmt.Exec(
				b.String(),
				msg.String(),
			); err != nil {
				return err
			}
		}
	}
	if err := stmt.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`insert into block_messages select * from mi on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	return tx.Commit()
}

func (st *storage) storeMpoolInclusions(msgs []api.MpoolUpdate) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
		create temp table mi (like mpool_messages excluding constraints) on commit drop;
	`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy mi (msg, add_ts) from stdin `)
	if err != nil {
		return err
	}

	for _, msg := range msgs {
		if msg.Type != api.MpoolAdd {
			continue
		}

		if _, err := stmt.Exec(
			msg.Message.Message.Cid().String(),
			time.Now().Unix(),
		); err != nil {
			return err
		}
	}

	if err := stmt.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`insert into mpool_messages select * from mi on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	return tx.Commit()
}

func (st *storage) storeDeals(deals map[string]api.MarketDeal) error {
	/*tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
		create temp table d (like deals excluding constraints) on commit drop;
	`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy d (id, pieceref, piecesize, client, "provider", "start", "end", epochprice, collateral) from stdin `)
	if err != nil {
		return err
	}

	var bloat uint64

	for id, deal := range deals {
		if len(deal.Proposal.PieceCID.String()) > 100 {
			bloat += uint64(len(deal.Proposal.PieceCID.String()))
			continue
		}
		if _, err := stmt.Exec(
			id,
			deal.Proposal.PieceCID.String(),
			deal.Proposal.PieceSize,
			deal.Proposal.Client.String(),
			deal.Proposal.Provider.String(),
			fmt.Sprint(deal.Proposal.StartEpoch),
			fmt.Sprint(deal.Proposal.EndEpoch),
			deal.Proposal.StoragePricePerEpoch.String(),
			deal.Proposal.ProviderCollateral.String(),
		); err != nil {
			return err
		}
	}
	if bloat > 0 {
		log.Warnf("deal PieceRefs had %d bytes of garbage", bloat)
	}

	if err := stmt.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`insert into deals select * from d on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Activations

	tx, err = st.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
		create temp table d (like deal_activations excluding constraints) on commit drop;
	`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	stmt, err = tx.Prepare(`copy d (deal, activation_epoch) from stdin `)
	if err != nil {
		return err
	}

	for id, deal := range deals {
		if deal.State.SectorStartEpoch <= 0 {
			continue
		}
		if _, err := stmt.Exec(
			id,
			deal.State.SectorStartEpoch,
		); err != nil {
			return err
		}
	}
	if err := stmt.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`insert into deal_activations select * from d on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	*/
	return nil
}

func (st *storage) refreshViews() error {
	if _, err := st.db.Exec(`refresh materialized view state_heights`); err != nil {
		return err
	}

	return nil
}

func (st *storage) close() error {
	return st.db.Close()
}
