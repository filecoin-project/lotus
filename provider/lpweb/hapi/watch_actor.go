package hapi

import (
	"context"
	"github.com/BurntSushi/toml"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	"golang.org/x/xerrors"
	"sort"
	"time"
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
	Addresses struct {
		MinerAddresses []string
	}
}

func (a *app) updateActor(ctx context.Context) error {
	a.rpcInfoLk.Lock()
	api := a.workingApi
	a.rpcInfoLk.Unlock()

	if api == nil {
		log.Warnw("no working api yet")
		return nil
	}

	var actorInfos []actorInfo

	confNameToAddr := map[address.Address][]string{} // address -> config names

	err := forEachConfig[minimalActorInfo](a, func(name string, info minimalActorInfo) error {
		for _, addr := range info.Addresses.MinerAddresses {
			a, err := address.NewFromString(addr)
			if err != nil {
				return xerrors.Errorf("parsing address: %w", err)
			}
			confNameToAddr[a] = append(confNameToAddr[a], name)
		}

		return nil
	})
	if err != nil {
		return err
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

		actorInfos = append(actorInfos, actorInfo{
			Address:              addr.String(),
			CLayers:              cnames,
			QualityAdjustedPower: types.DeciStr(p.MinerPower.QualityAdjPower),
			RawBytePower:         types.DeciStr(p.MinerPower.RawBytePower),
			Deadlines:            outDls,
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
