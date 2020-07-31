package syncer

import (
	"container/list"
	"context"
	"database/sql"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

var log = logging.Logger("syncer")

type Syncer struct {
	db *sql.DB

	headerLk sync.Mutex
	node     api.FullNode
}

func NewSyncer(db *sql.DB, node api.FullNode) *Syncer {
	return &Syncer{
		db:   db,
		node: node,
	}
}

func (s *Syncer) setupSchemas() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
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
	synced_at int not null,
	processed_at int
);

create unique index if not exists blocks_synced_cid_uindex
	on blocks_synced (cid,processed_at);

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
	on blocks (cid,height);

create materialized view if not exists state_heights
    as select distinct height, parentstateroot from blocks;

create index if not exists state_heights_height_index
	on state_heights (height);

create index if not exists state_heights_parentstateroot_index
	on state_heights (parentstateroot);
`); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Syncer) Start(ctx context.Context) {
	log.Debug("Starting Syncer")

	if err := s.setupSchemas(); err != nil {
		log.Fatal(err)
	}

	// doing the initial sync here lets us avoid the HCCurrent case in the switch
	head, err := s.node.ChainHead(ctx)
	if err != nil {
		log.Fatalw("Failed to get chain head form lotus", "error", err)
	}

	unsynced, err := s.unsyncedBlocks(ctx, head, time.Unix(0, 0))
	if err != nil {
		log.Fatalw("failed to gather unsynced blocks", "error", err)
	}

	if err := s.storeHeaders(unsynced, true, time.Now()); err != nil {
		log.Fatalw("failed to store unsynced blocks", "error", err)
	}

	// continue to keep the block headers table up to date.
	notifs, err := s.node.ChainNotify(ctx)
	if err != nil {
		log.Fatal(err)
	}

	lastSynced := time.Now()
	go func() {
		for notif := range notifs {
			for _, change := range notif {
				switch change.Type {
				case store.HCApply:
					unsynced, err := s.unsyncedBlocks(ctx, change.Val, lastSynced)
					if err != nil {
						log.Errorw("failed to gather unsynced blocks", "error", err)
					}

					if len(unsynced) == 0 {
						continue
					}

					if err := s.storeHeaders(unsynced, true, lastSynced); err != nil {
						// so this is pretty bad, need some kind of retry..
						// for now just log an error and the blocks will be attempted again on next notifi
						log.Errorw("failed to store unsynced blocks", "error", err)
					}

					lastSynced = time.Now()
				case store.HCRevert:
					log.Debug("revert todo")
				}
			}
		}
	}()
}

func (s *Syncer) unsyncedBlocks(ctx context.Context, head *types.TipSet, since time.Time) (map[cid.Cid]*types.BlockHeader, error) {
	// get a list of blocks we have already synced in the past 3 mins. This ensures we aren't returning the entire
	// table every time.
	lookback := since.Add(-(time.Minute * 3))
	log.Debugw("Gathering unsynced blocks", "since", lookback.String())
	hasList, err := s.syncedBlocks(lookback)
	if err != nil {
		return nil, err
	}

	// build a list of blocks that we have not synced.
	toVisit := list.New()
	for _, header := range head.Blocks() {
		toVisit.PushBack(header)
	}

	toSync := map[cid.Cid]*types.BlockHeader{}

	for toVisit.Len() > 0 {
		bh := toVisit.Remove(toVisit.Back()).(*types.BlockHeader)
		_, has := hasList[bh.Cid()]
		if _, seen := toSync[bh.Cid()]; seen || has {
			continue
		}

		toSync[bh.Cid()] = bh
		if len(toSync)%500 == 10 {
			log.Debugw("To visit", "toVisit", toVisit.Len(), "toSync", len(toSync), "current_height", bh.Height)
		}

		if len(bh.Parents) == 0 {
			continue
		}

		pts, err := s.node.ChainGetTipSet(ctx, types.NewTipSetKey(bh.Parents...))
		if err != nil {
			log.Error(err)
			continue
		}

		for _, header := range pts.Blocks() {
			toVisit.PushBack(header)
		}
	}
	log.Debugw("Gathered unsynced blocks", "count", len(toSync))
	return toSync, nil
}

func (s *Syncer) syncedBlocks(timestamp time.Time) (map[cid.Cid]struct{}, error) {
	// timestamp is used to return a configurable amount of rows based on when they were last added.
	rws, err := s.db.Query(`select cid FROM blocks_synced where synced_at > $1`, timestamp.Unix())
	if err != nil {
		return nil, xerrors.Errorf("Failed to query blocks_synced: %w", err)
	}
	out := map[cid.Cid]struct{}{}

	for rws.Next() {
		var c string
		if err := rws.Scan(&c); err != nil {
			return nil, xerrors.Errorf("Failed to scan blocks_synced: %w", err)
		}

		ci, err := cid.Parse(c)
		if err != nil {
			return nil, xerrors.Errorf("Failed to parse blocks_synced: %w", err)
		}

		out[ci] = struct{}{}
	}
	return out, nil
}

func (s *Syncer) storeHeaders(bhs map[cid.Cid]*types.BlockHeader, sync bool, timestamp time.Time) error {
	s.headerLk.Lock()
	defer s.headerLk.Unlock()
	if len(bhs) == 0 {
		return nil
	}
	log.Debugw("Storing Headers", "count", len(bhs))

	tx, err := s.db.Begin()
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

		stmt, err := tx.Prepare(`copy bs (cid, synced_at) from stdin `)
		if err != nil {
			return err
		}

		for _, bh := range bhs {
			if _, err := stmt.Exec(bh.Cid().String(), timestamp.Unix()); err != nil {
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
