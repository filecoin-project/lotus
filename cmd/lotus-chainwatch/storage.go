package main

import (
	"database/sql"
	"fmt"
	"github.com/filecoin-project/lotus/api"
	"golang.org/x/xerrors"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	_ "github.com/lib/pq"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
)

type storage struct {
	db *sql.DB

	headerLk sync.Mutex
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
create table if not exists blocks_synced
(
	cid text not null
		constraint blocks_synced_pk
			primary key,
	add_ts int not null
);

create unique index if not exists blocks_synced_cid_uindex
	on blocks_synced (cid);

create table if not exists block_parents
(
	block text not null,
	parent text not null
);

create unique index if not exists block_parents_block_parent_uindex
	on block_parents (block, parent);

create table if not exists blocks
(
	cid text not null
		constraint blocks_pk
			primary key,
	parentWeight numeric not null,
	parentStateRoot text not null,
	height bigint not null,
	miner text not null,
	timestamp bigint not null,
	vrfproof bytea,
	tickets bigint not null,
	eprof bytea,
	prand bytea,
	ep0partial bytea,
	ep0sector bigint not null,
	ep0challangei bigint not null
);

create unique index if not exists block_cid_uindex
	on blocks (cid);

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

create table if not exists messages
(
	cid text not null
		constraint messages_pk
			primary key,
	"from" text not null,
	"to" text not null,
	nonce int not null,
	value text not null,
	gasprice int not null,
	gaslimit int not null,
	method int,
	params bytea
);

create unique index if not exists messages_cid_uindex
	on messages (cid);
	
create table if not exists block_messages
(
	block text not null,
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

create table if not exists miner_heads
(
	head text not null,
	addr text not null,
	stateroot text not null,
	sectorset text not null,
	setsize decimal not null,
	provingset text not null,
	provingsize decimal not null,
	owner text not null,
	worker text not null,
	peerid text not null,
	sectorsize bigint not null,
	power bigint not null,
	active bool,
	ppe bigint not null,
	slashed_at bigint not null,
	constraint miner_heads_pk
		primary key (head, addr)
);

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

func (st *storage) storeActors(actors map[address.Address]map[types.Actor]cid.Cid) error {
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
			if _, err := stmt.Exec(addr.String(), act.Code.String(), act.Head.String(), act.Nonce, act.Balance.String(), st.String()); err != nil {
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

	return tx.Commit()
}

func (st *storage) storeMiners(miners map[minerKey]*minerInfo) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`

create temp table mh (like miner_heads excluding constraints) on commit drop;


`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy mh (head, addr, stateroot, sectorset, setsize, provingset, provingsize, owner, worker, peerid, sectorsize, power, active, ppe, slashed_at) from STDIN`)
	if err != nil {
		return err
	}
	for k, i := range miners {
		if _, err := stmt.Exec(
			k.act.Head.String(),
			k.addr.String(),
			k.stateroot.String(),
			i.state.Sectors.String(),
			fmt.Sprint(i.ssize),
			i.state.ProvingSet.String(),
			fmt.Sprint(i.psize),
			i.info.Owner.String(),
			i.info.Worker.String(),
			i.info.PeerID.String(),
			i.info.SectorSize,
			i.state.Power.String(),
			i.state.Active,
			i.state.ElectionPeriodStart,
			i.state.SlashedAt,
		); err != nil {
			return err
		}
	}
	if err := stmt.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`insert into miner_heads select * from mh on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

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

create temp table tbp (like block_parents excluding constraints) on commit drop;
create temp table bs (like blocks_synced excluding constraints) on commit drop;
create temp table b (like blocks excluding constraints) on commit drop;


`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

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

	stmt2, err := tx.Prepare(`copy b (cid, parentWeight, parentStateRoot, height, miner, "timestamp", vrfproof, tickets, eprof, prand, ep0partial, ep0sector, ep0challangei) from stdin `)
	if err != nil {
		return err
	}

	for _, bh := range bhs {
		l := len(bh.EPostProof.Candidates)
		if len(bh.EPostProof.Candidates) == 0 {
			bh.EPostProof.Candidates = append(bh.EPostProof.Candidates, types.EPostTicket{})
		}

		if _, err := stmt2.Exec(
			bh.Cid().String(),
			bh.ParentWeight.String(),
			bh.ParentStateRoot.String(),
			bh.Height,
			bh.Miner.String(),
			bh.Timestamp,
			bh.Ticket.VRFProof,
			l,
			bh.EPostProof.Proof,
			bh.EPostProof.PostRand,
			bh.EPostProof.Candidates[0].Partial,
			bh.EPostProof.Candidates[0].SectorID,
			bh.EPostProof.Candidates[0].ChallengeIndex); err != nil {
			log.Error(err)
		}
	}

	if err := stmt2.Close(); err != nil {
		return xerrors.Errorf("s2 close: %w", err)
	}

	if _, err := tx.Exec(`insert into blocks select * from b on conflict do nothing `); err != nil {
		return xerrors.Errorf("blk put: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return xerrors.Errorf("commit: %w", err)
	}
	return nil
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
			m.GasLimit.String(),
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
			m.GasUsed.String(),
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

func (st *storage) close() error {
	return st.db.Close()
}
