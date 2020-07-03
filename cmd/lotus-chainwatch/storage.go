package main

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/libp2p/go-libp2p-core/peer"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	miner_spec "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/ipfs/go-cid"
	_ "github.com/lib/pq"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

type storage struct {
	db *sql.DB

	headerLk sync.Mutex

	// stateful miner data
	minerSectors map[cid.Cid]struct{}
}

func openStorage(dbSource string) (*storage, error) {
	db, err := sql.Open("postgres", dbSource)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1350)

	ms := make(map[cid.Cid]struct{})
	ms[cid.Undef] = struct{}{}

	st := &storage{db: db, minerSectors: ms}

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

/* used to tell when a miners sectors (proven-not-yet-expired) changed if the miner_sectors_cid's are different a new sector was added or removed (terminated/expired) */
create table if not exists miner_sectors_heads
(
	miner_id text not null,
	miner_sectors_cid text not null,
	
	state_root text not null,	
	
	constraint miner_sectors_heads_pk
		primary key (miner_id,miner_sectors_cid)
    
);

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

type minerSectorUpdate struct {
	minerState *minerStateInfo
	tskey      types.TipSetKey
	oldSector  cid.Cid
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

	var updateMiners []*minerSectorUpdate
	for tsk, miners := range minerTips {
		for _, miner := range miners {
			sectorCID, err := st.getLatestMinerSectorCID(context.TODO(), miner.addr)
			if err != nil {
				panic(err)
			}
			if sectorCID == cid.Undef {
				continue
			}
			if _, found := st.minerSectors[sectorCID]; !found {
				// schedule miner table update
				updateMiners = append(updateMiners, &minerSectorUpdate{
					minerState: miner,
					tskey:      tsk,
					oldSector:  sectorCID,
				})
			}
			st.minerSectors[sectorCID] = struct{}{}
			log.Debugw("got sector CID", "miner", miner.addr, "cid", sectorCID.String())
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

	if err := tx.Commit(); err != nil {
		return err
	}
	return st.updateMinerSectors(updateMiners, api)
}

type deletedSector struct {
	deletedSector miner_spec.SectorOnChainInfo
	miner         address.Address
	tskey         types.TipSetKey
}

func (st *storage) updateMinerSectors(miners []*minerSectorUpdate, api api.FullNode) error {
	log.Info("updating miners constant sector table")
	var deletedSectors []*deletedSector
	for _, miner := range miners {
		s := &apiIpldStore{context.TODO(), api}
		newSectors, err := adt.AsArray(s, miner.minerState.state.Sectors)
		if err != nil {
			log.Warnw("new sectors as array", "error", err, "cid", miner.minerState.state.Sectors)
			return err
		}

		oldSectors, err := adt.AsArray(s, miner.oldSector)
		if err != nil {
			log.Warnw("old sectors as array", "error", err, "cid", miner.oldSector.String())
			return err
		}

		var oldSecInfo miner_spec.SectorOnChainInfo
		var newSecInfo miner_spec.SectorOnChainInfo
		// if we cannot find an old sector in the new list then it was removed.
		if err := oldSectors.ForEach(&oldSecInfo, func(i int64) error {
			found, err := newSectors.Get(uint64(oldSecInfo.Info.SectorNumber), &newSecInfo)
			if err != nil {
				log.Warnw("new sectors get", "error", err)
				return err
			}
			if !found {
				log.Infow("MINER DELETED SECTOR", "miner", miner.minerState.addr.String(), "sector", oldSecInfo.Info.SectorNumber, "tipset", miner.tskey.String())
				deletedSectors = append(deletedSectors, &deletedSector{
					deletedSector: oldSecInfo,
					miner:         miner.minerState.addr,
					tskey:         miner.tskey,
				})
			}
			return nil
		}); err != nil {
			log.Warnw("old sectors foreach", "error", err)
			return err
		}
		if len(deletedSectors) > 0 {
			log.Infow("Calculated updates", "miner", miner.minerState.addr, "deleted sectors", len(deletedSectors))
		}
	}
	// now we have all the sectors that were removed, update the database
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`UPDATE miner_sectors SET termination_epoch=$1 WHERE miner_id=$2 AND sector_id=$3`)
	if err != nil {
		return err
	}
	for _, ds := range deletedSectors {
		ts, err := api.ChainGetTipSet(context.TODO(), ds.tskey)
		if err != nil {
			log.Warnw("get tipset", "error", err)
			return err
		}
		// TODO validate this shits right
		if ts.Height() >= ds.deletedSector.Info.Expiration {
			// means it expired, do nothing
			log.Infow("expired sector", "miner", ds.miner.String(), "sector", ds.deletedSector.Info.SectorNumber)
			continue
		}
		log.Infow("terminated sector", "miner", ds.miner.String(), "sector", ds.deletedSector.Info.SectorNumber)
		// means it was terminated.
		if _, err := stmt.Exec(int64(ts.Height()), ds.miner.String(), int64(ds.deletedSector.Info.SectorNumber)); err != nil {
			return err
		}
	}

	if err := stmt.Close(); err != nil {
		return err
	}
	defer log.Info("update miner sectors complete")
	return tx.Commit()
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

func (st *storage) getLatestMinerSectorCID(ctx context.Context, miner address.Address) (cid.Cid, error) {
	queryStr := fmt.Sprintf(`
SELECT miner_sectors_cid
FROM miner_sectors_heads
LEFT JOIN blocks ON miner_sectors_heads.state_root = blocks.parentstateroot 
WHERE miner_id = '%s'
ORDER BY blocks.height DESC
LIMIT 1;
`,
		miner.String())

	var cidstr string
	err := st.db.QueryRowContext(ctx, queryStr).Scan(&cidstr)
	switch {
	case err == sql.ErrNoRows:
		log.Warnf("no miner with miner_id: %s in table", miner)
		return cid.Undef, nil
	case err != nil:
		return cid.Undef, err
	default:
		return cid.Decode(cidstr)
	}
}
