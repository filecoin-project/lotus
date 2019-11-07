package sector

import (
	"bytes"
	"context"
	"fmt"
	"github.com/filecoin-project/go-sectorbuilder/sealing_state"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	logging "github.com/ipfs/go-log"
	"golang.org/x/xerrors"
	"io"
	"math/bits"
	"sync"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/cborrpc"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var log = logging.Logger("sectorstore")

var sectorDealsPrefix = datastore.NewKey("/sectordeals")

type DealMapping struct {
	DealIDs   []uint64
	Allocated uint64
	Committed bool
}

type TicketFn func(context.Context) (*sectorbuilder.SealTicket, error)

// TODO: eventually handle sector storage here instead of in rust-sectorbuilder
type Store struct {
	sb    *sectorbuilder.SectorBuilder
	tktFn TicketFn

	dealsLk sync.Mutex
	deals   datastore.Datastore
}

func NewStore(sb *sectorbuilder.SectorBuilder, ds dtypes.MetadataDS, tktFn TicketFn) *Store {
	return &Store{
		sb:    sb,
		tktFn: tktFn,
		deals: namespace.Wrap(ds, sectorDealsPrefix),
	}
}

func (s *Store) SectorStatus(sid uint64) (*sectorbuilder.SectorSealingStatus, error) {
	status, err := s.sb.SealStatus(sid)
	if err != nil {
		return nil, err
	}

	return &status, nil
}

func (s *Store) AddPiece(ref string, size uint64, r io.Reader, dealIDs ...uint64) (sectorID uint64, err error) {
	sectorID, err = s.sb.AddPiece(ref, size, r)
	if err != nil {
		return 0, err
	}

	s.dealsLk.Lock()
	defer s.dealsLk.Unlock()

	k := datastore.NewKey(fmt.Sprint(sectorID))
	e, err := s.deals.Get(k)
	var deals DealMapping
	switch err {
	case nil:
		if err := cborrpc.ReadCborRPC(bytes.NewReader(e), &deals); err != nil {
			return 0, err
		}
		if deals.Committed {
			return 0, xerrors.Errorf("sector %d already committed", sectorID)
		}
		fallthrough
	case datastore.ErrNotFound:
		deals.DealIDs = append(deals.DealIDs, dealIDs...)
		deals.Allocated += size

		d, err := cborrpc.Dump(&deals)
		if err != nil {
			return 0, err
		}
		if err := s.deals.Put(k, d); err != nil {
			return 0, err
		}
	default:
		return 0, err
	}

	return sectorID, nil
}

func (s *Store) PieceSizesToFill(sectorID uint64) ([]uint64, error) {
	s.dealsLk.Lock()
	defer s.dealsLk.Unlock()

	k := datastore.NewKey(fmt.Sprint(sectorID))
	e, err := s.deals.Get(k)
	if err != nil {
		return nil, err
	}
	var info DealMapping
	if err := cborrpc.ReadCborRPC(bytes.NewReader(e), &info); err != nil {
		return nil, err
	}
	if info.Allocated > s.sb.SectorSize() {
		return nil, xerrors.Errorf("more data allocated in sector than should be able to fit: %d > %d", info.Allocated, s.sb.SectorSize())
	}

	return fillersFromRem(sectorbuilder.UserBytesForSectorSize(s.sb.SectorSize()) - info.Allocated)
}

func fillersFromRem(toFill uint64) ([]uint64, error) {
	toFill += toFill / 127 // convert to in-sector bytes for easier math

	out := make([]uint64, bits.OnesCount64(toFill))
	for i := range out {
		next := bits.TrailingZeros64(toFill)
		psize := uint64(1) << next
		toFill ^= psize
		out[i] = sectorbuilder.UserBytesForSectorSize(psize)
	}
	return out, nil
}

func (s *Store) DealsForCommit(sectorID uint64, commit bool) ([]uint64, error) {
	s.dealsLk.Lock()
	defer s.dealsLk.Unlock()

	k := datastore.NewKey(fmt.Sprint(sectorID))
	e, err := s.deals.Get(k)

	switch err {
	case nil:
		var deals DealMapping
		if err := cborrpc.ReadCborRPC(bytes.NewReader(e), &deals); err != nil {
			return nil, err
		}
		if !commit {
			return nil, nil
		}

		if deals.Committed {
			log.Errorf("getting deal IDs for sector %d: sector already marked as committed", sectorID)
		}

		deals.Committed = true
		d, err := cborrpc.Dump(&deals)
		if err != nil {
			return nil, err
		}
		if err := s.deals.Put(k, d); err != nil {
			return nil, err
		}

		return deals.DealIDs, nil
	case datastore.ErrNotFound:
		log.Errorf("getting deal IDs for sector %d failed: %s", err)
		return []uint64{}, nil
	default:
		return nil, err
	}
}

func (s *Store) SealPreCommit(ctx context.Context, sectorID uint64) (sectorbuilder.SealPreCommitOutput, error) {
	tkt, err := s.tktFn(ctx)
	if err != nil {
		return sectorbuilder.SealPreCommitOutput{}, err
	}

	return s.sb.SealPreCommit(sectorID, *tkt)
}

func (s *Store) SealComputeProof(ctx context.Context, sectorID uint64, height uint64, rand []byte) ([]byte, error) {
	var tick [32]byte
	copy(tick[:], rand)

	sco, err := s.sb.SealCommit(sectorID, sectorbuilder.SealSeed{
		BlockHeight: height,
		TicketBytes: tick,
	})
	if err != nil {
		return nil, err
	}
	return sco.Proof, nil
}

func (s *Store) Committed() ([]sectorbuilder.SectorSealingStatus, error) {
	l, err := s.sb.GetAllStagedSectors()
	if err != nil {
		return nil, err
	}

	out := make([]sectorbuilder.SectorSealingStatus, 0)
	for _, sid := range l {
		status, err := s.sb.SealStatus(sid)
		if err != nil {
			return nil, err
		}

		if status.State != sealing_state.Committed {
			continue
		}
		out = append(out, status)
	}

	return out, nil
}

func (s *Store) RunPoSt(ctx context.Context, sectors []*api.SectorInfo, r []byte, faults []uint64) ([]byte, error) {
	sbsi := make([]sectorbuilder.SectorInfo, len(sectors))
	for k, sector := range sectors {
		var commR [sectorbuilder.CommLen]byte
		if copy(commR[:], sector.CommR) != sectorbuilder.CommLen {
			return nil, xerrors.Errorf("commR too short, %d bytes", len(sector.CommR))
		}

		sbsi[k] = sectorbuilder.SectorInfo{
			SectorID: sector.SectorID,
			CommR:    commR,
		}
	}

	ssi := sectorbuilder.NewSortedSectorInfo(sbsi)

	var seed [sectorbuilder.CommLen]byte
	if copy(seed[:], r) != sectorbuilder.CommLen {
		return nil, xerrors.Errorf("random seed too short, %d bytes", len(r))
	}

	return s.sb.GeneratePoSt(ssi, seed, faults)
}
