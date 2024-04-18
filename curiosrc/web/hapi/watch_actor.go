package hapi

import (
	"context"
	"sort"
	"time"

	"github.com/BurntSushi/toml"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

const watchInterval = time.Second * 10

func (a *app) watchActor() {
	ticker := time.NewTicker(watchInterval)
	for {
		err := a.updateActor(context.TODO())
		if err != nil {
			log.Errorw("updating rpc info", "error", err)
		}
		select {
		case <-ticker.C:
		}
	}
}

type minimalActorInfo struct {
	Addresses []struct {
		MinerAddresses []string
	}
}

var startedAt = time.Now()

func (a *app) updateActor(ctx context.Context) error {
	a.rpcInfoLk.Lock()
	api := a.workingApi
	a.rpcInfoLk.Unlock()

	stor := store.ActorStore(ctx, blockstore.NewReadCachedBlockstore(blockstore.NewAPIBlockstore(a.workingApi), ChainBlockCache))

	if api == nil {
		if time.Since(startedAt) > time.Second*10 {
			log.Warnw("no working api yet")
		}
		return nil
	}

	var actorInfos []actorInfo

	confNameToAddr := map[address.Address][]string{} // address -> config names

	err := forEachConfig[minimalActorInfo](a, func(name string, info minimalActorInfo) error {
		for _, aset := range info.Addresses {
			for _, addr := range aset.MinerAddresses {
				a, err := address.NewFromString(addr)
				if err != nil {
					return xerrors.Errorf("parsing address: %w", err)
				}
				confNameToAddr[a] = append(confNameToAddr[a], name)
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	wins, err := a.spWins(ctx)
	if err != nil {
		return xerrors.Errorf("getting sp wins: %w", err)
	}

	for addr, cnames := range confNameToAddr {
		p, err := api.StateMinerPower(ctx, addr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting miner power: %w", err)
		}

		dls, err := api.StateMinerDeadlines(ctx, addr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting deadlines: %w", err)
		}

		mact, err := api.StateGetActor(ctx, addr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting actor: %w", err)
		}

		mas, err := miner.Load(stor, mact)
		if err != nil {
			return err
		}

		outDls := []actorDeadline{}

		for dlidx := range dls {
			p, err := api.StateMinerPartitions(ctx, addr, uint64(dlidx), types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("getting partition: %w", err)
			}

			dl := actorDeadline{
				Empty:      false,
				Current:    false, // todo
				Proven:     false,
				PartFaulty: false,
				Faulty:     false,
			}

			var live, faulty uint64

			for _, part := range p {
				l, err := part.LiveSectors.Count()
				if err != nil {
					return xerrors.Errorf("getting live sectors: %w", err)
				}
				live += l

				f, err := part.FaultySectors.Count()
				if err != nil {
					return xerrors.Errorf("getting faulty sectors: %w", err)
				}
				faulty += f
			}

			dl.Empty = live == 0
			dl.Proven = live > 0 && faulty == 0
			dl.PartFaulty = faulty > 0
			dl.Faulty = faulty > 0 && faulty == live

			outDls = append(outDls, dl)
		}

		pd, err := api.StateMinerProvingDeadline(ctx, addr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting proving deadline: %w", err)
		}

		if len(outDls) != 48 {
			return xerrors.Errorf("expected 48 deadlines, got %d", len(outDls))
		}

		outDls[pd.Index].Current = true

		avail, err := mas.AvailableBalance(mact.Balance)
		if err != nil {
			return xerrors.Errorf("getting available balance: %w", err)
		}

		mi, err := mas.Info()
		if err != nil {
			return xerrors.Errorf("getting miner info: %w", err)
		}

		wbal, err := api.WalletBalance(ctx, mi.Worker)
		if err != nil {
			return xerrors.Errorf("getting worker balance: %w", err)
		}

		sort.Strings(cnames)

		actorInfos = append(actorInfos, actorInfo{
			Address:              addr.String(),
			CLayers:              cnames,
			QualityAdjustedPower: types.DeciStr(p.MinerPower.QualityAdjPower),
			RawBytePower:         types.DeciStr(p.MinerPower.RawBytePower),
			Deadlines:            outDls,

			ActorBalance:   types.FIL(mact.Balance).Short(),
			ActorAvailable: types.FIL(avail).Short(),
			WorkerBalance:  types.FIL(wbal).Short(),

			Win1:  wins[addr].Win1, // note: zero values are fine here
			Win7:  wins[addr].Win7,
			Win30: wins[addr].Win30,
		})
	}

	sort.Slice(actorInfos, func(i, j int) bool {
		return actorInfos[i].Address < actorInfos[j].Address
	})

	a.actorInfoLk.Lock()
	a.actorInfos = actorInfos
	a.actorInfoLk.Unlock()

	return nil
}

func (a *app) loadConfigs(ctx context.Context) (map[string]string, error) {
	rows, err := a.db.Query(ctx, `SELECT title, config FROM harmony_config`)
	if err != nil {
		return nil, xerrors.Errorf("getting db configs: %w", err)
	}

	configs := make(map[string]string)
	for rows.Next() {
		var title, config string
		if err := rows.Scan(&title, &config); err != nil {
			return nil, xerrors.Errorf("scanning db configs: %w", err)
		}
		configs[title] = config
	}

	return configs, nil
}

type wins struct {
	SpID  int64 `db:"sp_id"`
	Win1  int64 `db:"win1"`
	Win7  int64 `db:"win7"`
	Win30 int64 `db:"win30"`
}

func (a *app) spWins(ctx context.Context) (map[address.Address]wins, error) {
	var w []wins

	// note: this query uses mining_tasks_won_sp_id_base_compute_time_index
	err := a.db.Select(ctx, &w, `WITH wins AS (
	    SELECT
	        sp_id,
	        base_compute_time,
	        won
	    FROM
	        mining_tasks
	    WHERE
	        won = true
	      AND base_compute_time > NOW() - INTERVAL '30 days'
	)

	SELECT
	    sp_id,
	    COUNT(*) FILTER (WHERE base_compute_time > NOW() - INTERVAL '1 day') AS "win1",
	    COUNT(*) FILTER (WHERE base_compute_time > NOW() - INTERVAL '7 days') AS "win7",
	    COUNT(*) FILTER (WHERE base_compute_time > NOW() - INTERVAL '30 days') AS "win30"
	FROM
	    wins
	GROUP BY
	    sp_id
	ORDER BY
	    sp_id`)
	if err != nil {
		return nil, xerrors.Errorf("query win counts: %w", err)
	}

	wm := make(map[address.Address]wins)
	for _, wi := range w {
		ma, err := address.NewIDAddress(uint64(wi.SpID))
		if err != nil {
			return nil, xerrors.Errorf("parsing miner address: %w", err)
		}

		wm[ma] = wi
	}

	return wm, nil
}

func forEachConfig[T any](a *app, cb func(name string, v T) error) error {
	confs, err := a.loadConfigs(context.Background())
	if err != nil {
		return err
	}

	for name, tomlStr := range confs {
		var info T
		if err := toml.Unmarshal([]byte(tomlStr), &info); err != nil {
			return xerrors.Errorf("unmarshaling %s config: %w", name, err)
		}

		if err := cb(name, info); err != nil {
			return xerrors.Errorf("cb: %w", err)
		}
	}

	return nil
}
