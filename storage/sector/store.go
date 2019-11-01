package sector

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/filecoin-project/go-sectorbuilder/sealing_state"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

func init() {
	cbor.RegisterCborType(dealMapping{})
}

var log = logging.Logger("sectorstore")

var sectorDealsPrefix = datastore.NewKey("/sectordeals")

type dealMapping struct {
	DealIDs   []uint64
	Committed bool
}

type TicketFn func(context.Context) (*sectorbuilder.SealTicket, error)

// TODO: eventually handle sector storage here instead of in rust-sectorbuilder
type Store struct {
	waitingLk sync.Mutex

	sb    *sectorbuilder.SectorBuilder
	tktFn TicketFn

	dealsLk sync.Mutex
	deals   datastore.Datastore

	waiting  map[uint64]chan struct{}
	incoming []chan sectorbuilder.SectorSealingStatus
	// TODO: outdated chan

	closeCh chan struct{}
}

func NewStore(sb *sectorbuilder.SectorBuilder, ds dtypes.MetadataDS, tktFn TicketFn) *Store {
	return &Store{
		sb:      sb,
		tktFn:   tktFn,
		deals:   namespace.Wrap(ds, sectorDealsPrefix),
		waiting: map[uint64]chan struct{}{},
		closeCh: make(chan struct{}),
	}
}

func (s *Store) restartSealing() {
	sectors, err := s.sb.GetAllStagedSectors()
	if err != nil {
		return
	}

	for _, sid := range sectors {
		status, err := s.sb.SealStatus(sid)
		if err != nil {
			return
		}

		if status.State != sealing_state.CommittingPaused { // TODO: Also handle PreCommit!
			continue
		}

		log.Infof("Sector %d is in paused state, resuming sealing", sid)
		go func() {
			// TODO: when we refactor wait-for-seal below, care about this output too
			//  (see SealSector below)
			_, err := s.sb.ResumeSealCommit(sid)
			if err != nil {
				return
			}
		}()
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

	s.waitingLk.Lock()
	_, exists := s.waiting[sectorID]
	if !exists { // pieces can share sectors
		s.waiting[sectorID] = make(chan struct{})
	}
	s.waitingLk.Unlock()

	s.dealsLk.Lock()
	defer s.dealsLk.Unlock()

	k := datastore.NewKey(fmt.Sprint(sectorID))
	e, err := s.deals.Get(k)
	var deals dealMapping
	switch err {
	case nil:
		if err := cbor.DecodeInto(e, &deals); err != nil {
			return 0, err
		}
		if deals.Committed {
			return 0, xerrors.Errorf("sector %d already committed", sectorID)
		}
		fallthrough
	case datastore.ErrNotFound:
		deals.DealIDs = append(deals.DealIDs, dealIDs...)
		d, err := cbor.DumpObject(&deals)
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

func (s *Store) DealsForCommit(sectorID uint64) ([]uint64, error) {
	s.dealsLk.Lock()
	defer s.dealsLk.Unlock()

	k := datastore.NewKey(fmt.Sprint(sectorID))
	e, err := s.deals.Get(k)

	switch err {
	case nil:
		var deals dealMapping
		if err := cbor.DecodeInto(e, &deals); err != nil {
			return nil, err
		}
		if deals.Committed {
			log.Errorf("getting deal IDs for sector %d: sector already marked as committed", sectorID)
		}

		deals.Committed = true
		d, err := cbor.DumpObject(&deals)
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

func (s *Store) SealPreCommit(ctx context.Context, sectorID uint64) error {
	tkt, err := s.tktFn(ctx)
	if err != nil {
		return err
	}

	// TODO: That's not async, is it?
	//  - If not then we probably can drop this wait-for-seal hack below
	_, err = s.sb.SealPreCommit(sectorID, *tkt)
	if err != nil {
		return err
	}
	return nil
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

func (s *Store) WaitSeal(ctx context.Context, sector uint64) (sectorbuilder.SectorSealingStatus, error) {
	s.waitingLk.Lock()
	watch, ok := s.waiting[sector]
	s.waitingLk.Unlock()
	if ok {
		select {
		case <-watch:
		case <-ctx.Done():
			return sectorbuilder.SectorSealingStatus{}, ctx.Err()
		}
	}

	return s.sb.SealStatus(sector)
}

func (s *Store) Commited() ([]sectorbuilder.SectorSealingStatus, error) {
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

func (s *Store) Stop() {
	close(s.closeCh)
}
