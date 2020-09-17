package processor

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/ipfs/go-cid"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/parmap"
)

func (p *Processor) toProcessBlocks(batch int) (map[cid.Cid]struct{}, error) {
	rows, err := p.db.Query(`
with toProcess as (
    select b.cid, b.height, rank() over (order by height) as rnk
    from blocks_synced bs
        left join blocks b on bs.cid = b.cid
    where bs.processed_at is null and b.height > 0
)
select cid
from toProcess
where rnk <= $1
`, batch)
	if err != nil {
		return nil, xerrors.Errorf("Failed to query for unprocessed blocks: %w", err)
	}
	out := map[cid.Cid]struct{}{}

	for rows.Next() {
		if rows.Err() != nil {
			return nil, err
		}
		var c string
		if err := rows.Scan(&c); err != nil {
			log.Errorf("Failed to scan unprocessed blocks: %s", err.Error())
			continue
		}
		ci, err := cid.Parse(c)
		if err != nil {
			log.Errorf("Failed to parse unprocessed blocks: %s", err.Error())
			continue
		}

		out[ci] = struct{}{}
	}

	return out, rows.Close()
}

func (p *Processor) fetchBlocks(ctx context.Context, bcs map[cid.Cid]struct{}) (
	blocks map[cid.Cid]*types.BlockHeader, minBlock abi.ChainEpoch, maxBlock abi.ChainEpoch) {
	var lk sync.Mutex
	blocks = map[cid.Cid]*types.BlockHeader{}
	minBlock = abi.ChainEpoch(math.MaxInt64)
	maxBlock = abi.ChainEpoch(0)

	parmap.Par(50, parmap.KMapArr(bcs), func(ci cid.Cid) {
		bh, err := p.node.ChainGetBlock(ctx, ci)
		if err != nil {
			// this is a pretty serious issue.
			log.Errorf("Failed to get block header %s: %s", ci.String(), err.Error())
		}

		lk.Lock()
		blocks[ci] = bh
		if bh.Height < minBlock {
			minBlock = bh.Height
		}
		if bh.Height > maxBlock {
			maxBlock = bh.Height
		}
		lk.Unlock()
	})

	return
}

func (p *Processor) unprocessedBlocks(ctx context.Context, batch int) (map[cid.Cid]*types.BlockHeader, error) {
	start := time.Now()
	defer func() {
		log.Debugw("Gathered Blocks to process", "duration", time.Since(start).String())
	}()

	bcs, err := p.toProcessBlocks(batch)
	if err != nil {
		return nil,err
	}

	out, minBlock, maxBlock := p.fetchBlocks(ctx, bcs)
	if minBlock <= maxBlock {
		log.Infow("Gathered Blocks to Process", "start", minBlock, "end", maxBlock)
	}
	return out, nil
}

func (p *Processor) markBlocksProcessed(ctx context.Context, processed map[cid.Cid]*types.BlockHeader) error {
	start := time.Now()
	processedHeight := abi.ChainEpoch(0)
	defer func() {
		log.Debugw("Marked blocks as Processed", "duration", time.Since(start).String())
		log.Infow("Processed Blocks", "height", processedHeight)
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

	for c, bh := range processed {
		if bh.Height > processedHeight {
			processedHeight = bh.Height
		}
		if _, err := stmt.Exec(processedAt, c.String()); err != nil {
			return err
		}
	}

	if err := stmt.Close(); err != nil {
		return err
	}

	return tx.Commit()
}
