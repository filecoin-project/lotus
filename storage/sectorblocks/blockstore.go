package sectorblocks

import (
	"context"
	"io/ioutil"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/storage/sealing"
)

var log = logging.Logger("sectorblocks")

type SectorBlockStore struct {
	intermediate blockstore.Blockstore
	sectorBlocks *SectorBlocks

	approveUnseal func() error
}

func (s *SectorBlockStore) DeleteBlock(cid.Cid) error {
	panic("not supported")
}
func (s *SectorBlockStore) GetSize(cid.Cid) (int, error) {
	panic("not supported")
}

func (s *SectorBlockStore) Put(blocks.Block) error {
	panic("not supported")
}

func (s *SectorBlockStore) PutMany([]blocks.Block) error {
	panic("not supported")
}

func (s *SectorBlockStore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	panic("not supported")
}

func (s *SectorBlockStore) HashOnRead(enabled bool) {
	panic("not supported")
}

func (s *SectorBlockStore) Has(c cid.Cid) (bool, error) {
	has, err := s.intermediate.Has(c)
	if err != nil {
		return false, err
	}
	if has {
		return true, nil
	}

	return s.sectorBlocks.Has(c)
}

func (s *SectorBlockStore) Get(c cid.Cid) (blocks.Block, error) {
	val, err := s.intermediate.Get(c)
	if err == nil {
		return val, nil
	}
	if err != blockstore.ErrNotFound {
		return nil, err
	}

	refs, err := s.sectorBlocks.GetRefs(c)
	if err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return nil, blockstore.ErrNotFound
	}

	// TODO: better strategy (e.g. look for already unsealed)
	var best api.SealedRef
	var bestSi sealing.SectorInfo
	for _, r := range refs {
		si, err := s.sectorBlocks.Miner.GetSectorInfo(r.SectorID)
		if err != nil {
			return nil, xerrors.Errorf("getting sector info: %w", err)
		}
		if si.State == api.Proving {
			best = r
			bestSi = si
			break
		}
	}
	if bestSi.State == api.UndefinedSectorState {
		return nil, xerrors.New("no sealed sector found")
	}

	log.Infof("reading block %s from sector %d(+%d;%d)", c, best.SectorID, best.Offset, best.Size)

	r, err := s.sectorBlocks.sb.ReadPieceFromSealedSector(
		context.TODO(),
		best.SectorID,
		best.Offset,
		best.Size,
		bestSi.Ticket.TicketBytes,
		bestSi.CommD,
	)
	if err != nil {
		return nil, xerrors.Errorf("unsealing block: %w", err)
	}
	defer r.Close()

	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, xerrors.Errorf("reading block data: %w", err)
	}
	if uint64(len(data)) != best.Size {
		return nil, xerrors.Errorf("got wrong amount of data: %d != !d", len(data), best.Size)
	}

	b, err := blocks.NewBlockWithCid(data, c)
	if err != nil {
		return nil, xerrors.Errorf("sbs get (%d[%d:%d]): %w", best.SectorID, best.Offset, best.Offset+best.Size, err)
	}

	return b, nil
}

var _ blockstore.Blockstore = &SectorBlockStore{}
