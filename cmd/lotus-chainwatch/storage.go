package main

import (
	"database/sql"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	_ "github.com/mattn/go-sqlite3"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
)

type storage struct {
	db *sql.DB

	headerLk sync.Mutex
}

func openStorage(dbSource string) (*storage, error) {
	db, err := sql.Open("sqlite3", dbSource)
	if err != nil {
		return nil, err
	}

	st := &storage{db: db}

	return st, st.setup()
}

func (st *storage) setup() error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
create table if not exists actors
  (
	id text not null,
	code text not null,
	head text not null,
	nonce int not null,
	balance text not null,
	stateroot text
		constraint actors_blocks_stateroot_fk
			references blocks (parentStateRoot),
	constraint actors_pk
		primary key (id, nonce, balance, stateroot)
  );
  
create index if not exists actors_id_index
	on actors (id);

create table if not exists id_address_map
(
	id text not null
		constraint id_address_map_actors_id_fk
			references actors (id),
	address text not null,
	constraint id_address_map_pk
		primary key (id, address)
);

create index if not exists id_address_map_address_index
	on id_address_map (address);

create index if not exists id_address_map_id_index
	on id_address_map (id);

create table if not exists messages
(
	cid text not null
		constraint messages_pk
			primary key,
	"from" text not null
		constraint messages_id_address_map_from_fk
			references id_address_map (address),
	"to" text not null
		constraint messages_id_address_map_to_fk
			references id_address_map (address),
	nonce int not null,
	value text not null,
	gasprice int not null,
	gaslimit int not null,
	method int,
	params blob
);

create unique index if not exists messages_cid_uindex
	on messages (cid);

create table if not exists blocks
(
	cid text not null
		constraint blocks_pk
			primary key,
	parentWeight numeric not null,
	parentStateRoot text not null,
	height int not null,
	miner text not null
		constraint blocks_id_address_map_miner_fk
			references id_address_map (address),
	timestamp int not null,
	vrfproof blob,
	tickets int not null,
	eprof blob,
	prand blob,
	ep0partial blob,
	ep0sector int not null,
	ep0challangei int not null
);

create unique index if not exists block_cid_uindex
	on blocks (cid);

create table if not exists blocks_synced
(
	cid text not null
		constraint blocks_synced_pk
			primary key
		constraint blocks_synced_blocks_cid_fk
			references blocks,
	add_ts int not null
);

create unique index if not exists blocks_synced_cid_uindex
	on blocks_synced (cid);

create table if not exists block_parents
(
	block text not null
		constraint block_parents_blocks_cid_fk
			references blocks,
	parent text not null
		constraint block_parents_blocks_cid_fk_2
			references blocks
);

create unique index if not exists block_parents_block_parent_uindex
	on block_parents (block, parent);

create unique index if not exists blocks_cid_uindex
	on blocks (cid);
	
create table if not exists block_messages
(
	block text not null
		constraint block_messages_blk_fk
			references blocks (cid),
	message text not null
		constraint block_messages_msg_fk
			references messages,
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
	msg text not null
		constraint receipts_messages_cid_fk
			references messages,
	state text not null
		constraint receipts_blocks_parentStateRoot_fk
			references blocks (parentStateRoot),
	idx int not null,
	exit int not null,
	gas_used int not null,
	return blob,
	constraint receipts_pk
		primary key (msg, state)
);

create index if not exists receipts_msg_state_index
	on receipts (msg, state);


create table if not exists miner_heads
(
	head text not null
		constraint miner_heads_actors_head_fk
			references actors (head),
	addr text not null
		constraint miner_heads_actors_id_fk
			references actors (id),
	stateroot text not null
		constraint miner_heads_blocks_stateroot_fk
			references blocks (parentStateRoot),
	sectorset text not null,
	setsize int not null,
	provingset text not null,
	provingsize int not null,
	owner text not null,
	worker text not null,
	peerid text not null,
	sectorsize int not null,
	power text not null,
	active int,
	ppe int not null,
	slashed_at int not null,
	constraint miner_heads_id_address_map_owner_fk
		foreign key (owner) references id_address_map (address),
	constraint miner_heads_id_address_map_worker_fk
		foreign key (worker) references id_address_map (address),
	constraint miner_heads_pk
		primary key (head, addr)
);

`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (st *storage) hasBlock(bh cid.Cid) bool {
	var exitsts bool
	err := st.db.QueryRow(`select exists (select 1 FROM blocks_synced where cid=?)`, bh.String()).Scan(&exitsts)
	if err != nil {
		log.Error(err)
		return false
	}
	return exitsts
}

func (st *storage) storeActors(actors map[address.Address]map[types.Actor]cid.Cid) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`insert into actors (id, code, head, nonce, balance, stateroot) values (?, ?, ?, ?, ?, ?) on conflict do nothing`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for addr, acts := range actors {
		for act, st := range acts {
			if _, err := stmt.Exec(addr.String(), act.Code.String(), act.Head.String(), act.Nonce, act.Balance.String(), st.String()); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (st *storage) storeMiners(miners map[minerKey]*minerInfo) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`insert into miner_heads (head, addr, stateroot, sectorset, setsize, provingset, provingsize, owner, worker, peerid, sectorsize, power, active, ppe, slashed_at) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) on conflict do nothing`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for k, i := range miners {
		if _, err := stmt.Exec(
			k.act.Head.String(),
			k.addr.String(),
			k.stateroot.String(),
			i.state.Sectors.String(),
			i.ssize,
			i.state.ProvingSet.String(),
			i.psize,
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

	return tx.Commit()
}

func (st *storage) storeHeaders(bhs map[cid.Cid]*types.BlockHeader, sync bool) error {
	st.headerLk.Lock()
	defer st.headerLk.Unlock()

	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`insert into blocks (cid, parentWeight, parentStateRoot, height, miner, "timestamp", vrfproof, tickets, eprof, prand, ep0partial, ep0sector, ep0challangei) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) on conflict do nothing`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, bh := range bhs {
		l := len(bh.EPostProof.Candidates)
		if len(bh.EPostProof.Candidates) == 0 {
			bh.EPostProof.Candidates = append(bh.EPostProof.Candidates, types.EPostTicket{})
		}

		if _, err := stmt.Exec(bh.Cid().String(),
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
			bh.EPostProof.Candidates[0].ChallengeIndex,
		); err != nil {
			return err
		}
	}

	stmt2, err := tx.Prepare(`insert into block_parents (block, parent) values (?, ?) on conflict do nothing`)
	if err != nil {
		return err
	}
	defer stmt2.Close()
	for _, bh := range bhs {
		for _, parent := range bh.Parents {
			if _, err := stmt2.Exec(bh.Cid().String(), parent.String()); err != nil {
				return err
			}
		}
	}

	if sync {
		stmt, err := tx.Prepare(`insert into blocks_synced (cid, add_ts) values (?, ?) on conflict do nothing`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		now := time.Now().Unix()

		for _, bh := range bhs {
			if _, err := stmt.Exec(bh.Cid().String(), now); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (st *storage) storeMessages(msgs map[cid.Cid]*types.Message) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`insert into messages (cid, "from", "to", nonce, "value", gasprice, gaslimit, method, params) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) on conflict do nothing`)
	if err != nil {
		return err
	}
	defer stmt.Close()

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

	return tx.Commit()
}

func (st *storage) storeReceipts(recs map[mrec]*types.MessageReceipt) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`insert into receipts (msg, state, idx, exit, gas_used, return) VALUES (?, ?, ?, ?, ?, ?) on conflict do nothing`)
	if err != nil {
		return err
	}
	defer stmt.Close()

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

	return tx.Commit()
}

func (st *storage) storeAddressMap(addrs map[address.Address]address.Address) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`insert into id_address_map (id, address) VALUES (?, ?) on conflict do nothing`)
	if err != nil {
		return err
	}
	defer stmt.Close()

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

	return tx.Commit()
}

func (st *storage) storeMsgInclusions(incls map[cid.Cid][]cid.Cid) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`insert into block_messages (block, message) VALUES (?, ?) on conflict do nothing`)
	if err != nil {
		return err
	}
	defer stmt.Close()

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

	return tx.Commit()
}

func (st *storage) storeMpoolInclusion(msg cid.Cid) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`insert into mpool_messages (msg, add_ts) VALUES (?, ?) on conflict do nothing`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	if _, err := stmt.Exec(
		msg.String(),
		time.Now().Unix(),
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (st *storage) close() error {
	return st.db.Close()
}
