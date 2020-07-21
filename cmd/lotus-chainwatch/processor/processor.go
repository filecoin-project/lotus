package processor

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/parmap"
)

var log = logging.Logger("processor")

type Processor struct {
	db *sql.DB

	node api.FullNode

	genesisTs *types.TipSet

	// number of blocks processed at a time
	batch int
}

type ActorTips map[types.TipSetKey][]actorInfo

type actorInfo struct {
	act types.Actor

	stateroot cid.Cid
	height    abi.ChainEpoch // so that we can walk the actor changes in chronological order.

	tsKey       types.TipSetKey
	parentTsKey types.TipSetKey

	addr  address.Address
	state string
}

func NewProcessor(db *sql.DB, node api.FullNode, batch int) *Processor {
	return &Processor{
		db:    db,
		node:  node,
		batch: batch,
	}
}

func (p *Processor) setupSchemas() error {
	if err := p.setupMarket(); err != nil {
		return err
	}

	if err := p.setupMiners(); err != nil {
		return err
	}

	if err := p.setupRewards(); err != nil {
		return err
	}

	if err := p.setupMessages(); err != nil {
		return err
	}

	if err := p.setupCommonActors(); err != nil {
		return err
	}

	return nil
}

func (p *Processor) Start(ctx context.Context) {
	log.Debug("Starting Processor")

	if err := p.setupSchemas(); err != nil {
		log.Fatalw("Failed to setup processor", "error", err)
	}

	var err error
	p.genesisTs, err = p.node.ChainGetGenesis(ctx)
	if err != nil {
		log.Fatalw("Failed to get genesis state from lotus", "error", err.Error())
	}

	go p.subMpool(ctx)

	// main processor loop
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Debugw("Stopping Processor...")
				return
			default:
				toProcess, err := p.unprocessedBlocks(ctx, p.batch)
				if err != nil {
					log.Fatalw("Failed to get unprocessed blocks", "error", err)
				}

				if len(toProcess) == 0 {
					log.Debugw("No unprocessed blocks. Wait then try again...")
					time.Sleep(time.Second * 10)
					continue
				}

				// TODO special case genesis state handling here to avoid all the special cases that will be needed for it else where
				// before doing "normal" processing.

				actorChanges, err := p.collectActorChanges(ctx, toProcess)
				if err != nil {
					log.Fatalw("Failed to collect actor changes", "error", err)
				}

				grp, ctx := errgroup.WithContext(ctx)

				grp.Go(func() error {
					if err := p.HandleMarketChanges(ctx, actorChanges[builtin.StorageMarketActorCodeID]); err != nil {
						return xerrors.Errorf("Failed to handle market changes: %w", err)
					}
					return nil
				})

				grp.Go(func() error {
					if err := p.HandleMinerChanges(ctx, actorChanges[builtin.StorageMinerActorCodeID]); err != nil {
						return xerrors.Errorf("Failed to handle miner changes: %w", err)
					}
					return nil
				})

				grp.Go(func() error {
					if err := p.HandleRewardChanges(ctx, actorChanges[builtin.RewardActorCodeID]); err != nil {
						return xerrors.Errorf("Failed to handle reward changes: %w", err)
					}
					return nil
				})

				grp.Go(func() error {
					if err := p.HandleMessageChanges(ctx, toProcess); err != nil {
						return xerrors.Errorf("Failed to handle message changes: %w", err)
					}
					return nil
				})

				grp.Go(func() error {
					if err := p.HandleCommonActorsChanges(ctx, actorChanges); err != nil {
						return xerrors.Errorf("Failed to handle common actor changes: %w", err)
					}
					return nil
				})

				if err := grp.Wait(); err != nil {
					log.Errorw("Failed to handle actor changes...retrying", "error", err)
					continue
				}

				if err := p.markBlocksProcessed(ctx, toProcess); err != nil {
					log.Fatalw("Failed to mark blocks as processed", "error", err)
				}

				if err := p.refreshViews(); err != nil {
					log.Errorw("Failed to refresh views", "error", err)
				}
			}
		}
	}()

}

func (p *Processor) refreshViews() error {
	if _, err := p.db.Exec(`refresh materialized view state_heights`); err != nil {
		return err
	}

	if _, err := p.db.Exec(`refresh materialized view miner_sectors_view`); err != nil {
		return err
	}

	return nil
}

func (p *Processor) collectActorChanges(ctx context.Context, toProcess map[cid.Cid]*types.BlockHeader) (map[cid.Cid]ActorTips, error) {
	start := time.Now()
	defer func() {
		log.Debugw("Collected Actor Changes", "duration", time.Since(start).String())
	}()
	// ActorCode - > tipset->[]actorInfo
	out := map[cid.Cid]ActorTips{}
	var outMu sync.Mutex

	// map of addresses to changed actors
	var changes map[string]types.Actor
	actorsSeen := map[cid.Cid]struct{}{}

	// collect all actor state that has changes between block headers
	paDone := 0
	parmap.Par(50, parmap.MapArr(toProcess), func(bh *types.BlockHeader) {
		paDone++
		if paDone%100 == 0 {
			log.Debugw("Collecting actor changes", "done", paDone, "percent", (paDone*100)/len(toProcess))
		}

		pts, err := p.node.ChainGetTipSet(ctx, types.NewTipSetKey(bh.Parents...))
		if err != nil {
			panic(err)
		}

		// collect all actors that had state changes between the blockheader parent-state and its grandparent-state.
		// TODO: changes will contain deleted actors, this causes needless processing further down the pipeline, consider
		// a separate strategy for deleted actors
		changes, err = p.node.StateChangedActors(ctx, pts.ParentState(), bh.ParentStateRoot)
		if err != nil {
			panic(err)
		}

		// record the state of all actors that have changed
		for a, act := range changes {
			act := act
			a := a

			addr, err := address.NewFromString(a)
			if err != nil {
				panic(err)
			}

			ast, err := p.node.StateReadState(ctx, addr, pts.Key())
			if err != nil {
				panic(err)
			}

			// TODO look here for an empty state, maybe thats a sign the actor was deleted?

			state, err := json.Marshal(ast.State)
			if err != nil {
				panic(err)
			}

			outMu.Lock()
			if _, ok := actorsSeen[act.Head]; !ok {
				_, ok := out[act.Code]
				if !ok {
					out[act.Code] = map[types.TipSetKey][]actorInfo{}
				}
				out[act.Code][pts.Key()] = append(out[act.Code][pts.Key()], actorInfo{
					act:         act,
					stateroot:   bh.ParentStateRoot,
					height:      bh.Height,
					tsKey:       pts.Key(),
					parentTsKey: pts.Parents(),
					addr:        addr,
					state:       string(state),
				})
			}
			actorsSeen[act.Head] = struct{}{}
			outMu.Unlock()
		}
	})
	return out, nil
}

func (p *Processor) unprocessedBlocks(ctx context.Context, batch int) (map[cid.Cid]*types.BlockHeader, error) {
	start := time.Now()
	defer func() {
		log.Debugw("Gathered Blocks to process", "duration", time.Since(start).String())
	}()
	rows, err := p.db.Query(`
with toProcess as (
    select blocks.cid, blocks.height, rank() over (order by height) as rnk
    from blocks
        left join blocks_synced bs on blocks.cid = bs.cid
    where bs.processed_at is null and blocks.height > 0
)
select cid
from toProcess
where rnk <= $1
`, batch)
	if err != nil {
		return nil, xerrors.Errorf("Failed to query for unprocessed blocks: %w", err)
	}
	out := map[cid.Cid]*types.BlockHeader{}

	// TODO consider parallel execution here for getting the blocks from the api as is done in fetchMessages()
	for rows.Next() {
		if rows.Err() != nil {
			return nil, err
		}
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, xerrors.Errorf("Failed to scan unprocessed blocks: %w", err)
		}
		ci, err := cid.Parse(c)
		if err != nil {
			return nil, xerrors.Errorf("Failed to parse unprocessed blocks: %w", err)
		}
		bh, err := p.node.ChainGetBlock(ctx, ci)
		if err != nil {
			// this is a pretty serious issue.
			return nil, xerrors.Errorf("Failed to get block header %s: %w", ci.String(), err)
		}
		out[ci] = bh
	}
	return out, rows.Close()
}

func (p *Processor) markBlocksProcessed(ctx context.Context, processed map[cid.Cid]*types.BlockHeader) error {
	start := time.Now()
	defer func() {
		log.Debugw("Marked blocks as Processed", "duration", time.Since(start).String())
	}()
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	processedAt := time.Now().Unix()
	stmt, err := tx.Prepare(`update blocks_synced set processed_at=$1 where cid=$2`)
	if err != nil {
		return err
	}

	for c := range processed {
		if _, err := stmt.Exec(processedAt, c.String()); err != nil {
			return err
		}
	}

	if err := stmt.Close(); err != nil {
		return err
	}

	return tx.Commit()
}
