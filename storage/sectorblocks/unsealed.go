package sectorblocks

import (
	"context"
	"sync"

	logging "github.com/ipfs/go-log"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/lib/sectorbuilder"
)

var log = logging.Logger("sectorblocks")

type unsealedBlocks struct {
	lk sync.Mutex
	sb *sectorbuilder.SectorBuilder

	// TODO: Treat this as some sort of cache, one with rather aggressive GC
	// TODO: This REALLY, REALLY needs to be on-disk
	unsealed map[string][]byte

	unsealing map[string]chan struct{}
}

func (ub *unsealedBlocks) getRef(ctx context.Context, refs []api.SealedRef, approveUnseal func() error) ([]byte, error) {
	var best api.SealedRef

	ub.lk.Lock()
	for _, ref := range refs {
		b, ok := ub.unsealed[ref.Piece]
		if ok {
			ub.lk.Unlock()
			return b[ref.Offset : ref.Offset+uint64(ref.Size)], nil
		}
		// TODO: pick unsealing based on how long it's running (or just select all relevant, usually it'll be just one)
		_, ok = ub.unsealing[ref.Piece]
		if ok {
			best = ref
			break
		}
		best = ref
	}
	ub.lk.Unlock()

	b, err := ub.maybeUnseal(ctx, best.Piece, approveUnseal)
	if err != nil {
		return nil, err
	}

	return b[best.Offset : best.Offset+uint64(best.Size)], nil
}

func (ub *unsealedBlocks) maybeUnseal(ctx context.Context, pieceKey string, approveUnseal func() error) ([]byte, error) {
	ub.lk.Lock()
	defer ub.lk.Unlock()

	out, ok := ub.unsealed[pieceKey]
	if ok {
		return out, nil
	}

	wait, ok := ub.unsealing[pieceKey]
	if ok {
		ub.lk.Unlock()
		select {
		case <-wait:
			ub.lk.Lock()
			// TODO: make sure this is not racy with gc when it's implemented
			return ub.unsealed[pieceKey], nil
		case <-ctx.Done():
			ub.lk.Lock()
			return nil, ctx.Err()
		}
	}

	// TODO: doing this under a lock is suboptimal.. but simpler
	err := approveUnseal()
	if err != nil {
		return nil, err
	}

	ub.unsealing[pieceKey] = make(chan struct{})
	ub.lk.Unlock()

	log.Infof("Unsealing piece '%s'", pieceKey)
	data, err := ub.sb.ReadPieceFromSealedSector(pieceKey)
	ub.lk.Lock()

	if err != nil {
		// TODO: tell subs
		log.Error(err)
		return nil, err
	}

	ub.unsealed[pieceKey] = data
	close(ub.unsealing[pieceKey])
	return data, nil
}
