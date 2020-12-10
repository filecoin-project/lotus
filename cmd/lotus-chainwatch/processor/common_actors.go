package processor

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"

	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	_init "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/events/state"
	"github.com/filecoin-project/lotus/chain/types"
	cw_util "github.com/filecoin-project/lotus/cmd/lotus-chainwatch/util"
)

func (p *Processor) setupCommonActors() error {
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
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

`); err != nil {
		return err
	}

	return tx.Commit()
}

func (p *Processor) HandleCommonActorsChanges(ctx context.Context, actors map[cid.Cid]ActorTips) error {
	if err := p.storeActorAddresses(ctx, actors); err != nil {
		return err
	}

	grp, _ := errgroup.WithContext(ctx)

	grp.Go(func() error {
		if err := p.storeActorHeads(actors); err != nil {
			return err
		}
		return nil
	})

	grp.Go(func() error {
		if err := p.storeActorStates(actors); err != nil {
			return err
		}
		return nil
	})

	return grp.Wait()
}

type UpdateAddresses struct {
	Old state.AddressPair
	New state.AddressPair
}

func (p Processor) storeActorAddresses(ctx context.Context, actors map[cid.Cid]ActorTips) error {
	start := time.Now()
	defer func() {
		log.Debugw("Stored Actor Addresses", "duration", time.Since(start).String())
	}()

	addressToID := map[address.Address]address.Address{}
	// HACK until genesis storage is figured out:
	addressToID[builtin2.SystemActorAddr] = builtin2.SystemActorAddr
	addressToID[builtin2.InitActorAddr] = builtin2.InitActorAddr
	addressToID[builtin2.RewardActorAddr] = builtin2.RewardActorAddr
	addressToID[builtin2.CronActorAddr] = builtin2.CronActorAddr
	addressToID[builtin2.StoragePowerActorAddr] = builtin2.StoragePowerActorAddr
	addressToID[builtin2.StorageMarketActorAddr] = builtin2.StorageMarketActorAddr
	addressToID[builtin2.VerifiedRegistryActorAddr] = builtin2.VerifiedRegistryActorAddr
	addressToID[builtin2.BurntFundsActorAddr] = builtin2.BurntFundsActorAddr
	initActor, err := p.node.StateGetActor(ctx, builtin2.InitActorAddr, types.EmptyTSK)
	if err != nil {
		return err
	}

	initActorState, err := _init.Load(cw_util.NewAPIIpldStore(ctx, p.node), initActor)
	if err != nil {
		return err
	}
	// gross..
	if err := initActorState.ForEachActor(func(id abi.ActorID, addr address.Address) error {
		idAddr, err := address.NewIDAddress(uint64(id))
		if err != nil {
			return err
		}
		addressToID[addr] = idAddr
		return nil
	}); err != nil {
		return err
	}
	tx, err := p.db.Begin()
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

	for a, i := range addressToID {
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

	// HACK until chain watch can handle reorgs we need to update this table when ID -> PubKey mappings change
	if _, err := tx.Exec(`insert into id_address_map select * from iam on conflict (id) do update set address = EXCLUDED.address`); err != nil {
		log.Warnw("Failed to update id_address_map table, this is a known issue")
		return nil
	}

	return tx.Commit()
}

func (p *Processor) storeActorHeads(actors map[cid.Cid]ActorTips) error {
	start := time.Now()
	defer func() {
		log.Debugw("Stored Actor Heads", "duration", time.Since(start).String())
	}()
	// Basic
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`
		create temp table a_tmp (like actors excluding constraints) on commit drop;
	`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy a_tmp (id, code, head, nonce, balance, stateroot) from stdin `)
	if err != nil {
		return err
	}

	for code, actTips := range actors {
		actorName := code.String()
		if builtin.IsBuiltinActor(code) {
			actorName = builtin.ActorNameByCode(code)
		}
		for _, actorInfo := range actTips {
			for _, a := range actorInfo {
				if _, err := stmt.Exec(a.addr.String(), actorName, a.act.Head.String(), a.act.Nonce, a.act.Balance.String(), a.stateroot.String()); err != nil {
					return err
				}
			}
		}
	}

	if err := stmt.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`insert into actors select * from a_tmp on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	return tx.Commit()
}

func (p *Processor) storeActorStates(actors map[cid.Cid]ActorTips) error {
	start := time.Now()
	defer func() {
		log.Debugw("Stored Actor States", "duration", time.Since(start).String())
	}()
	// States
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`
		create temp table as_tmp (like actor_states excluding constraints) on commit drop;
	`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy as_tmp (head, code, state) from stdin `)
	if err != nil {
		return err
	}

	for code, actTips := range actors {
		actorName := code.String()
		if builtin.IsBuiltinActor(code) {
			actorName = builtin.ActorNameByCode(code)
		}
		for _, actorInfo := range actTips {
			for _, a := range actorInfo {
				if _, err := stmt.Exec(a.act.Head.String(), actorName, a.state); err != nil {
					return err
				}
			}
		}
	}

	if err := stmt.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`insert into actor_states select * from as_tmp on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	return tx.Commit()
}
