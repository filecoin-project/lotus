package sector

import (
	"context"
	"golang.org/x/xerrors"
	"io"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/lib/sectorbuilder"

	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("sectorstore")

// TODO: eventually handle sector storage here instead of in rust-sectorbuilder
type Store struct {
	lk sync.Mutex
	sb *sectorbuilder.SectorBuilder

	waiting  map[uint64]chan struct{}
	incoming []chan sectorbuilder.SectorSealingStatus
	// TODO: outdated chan

	closeCh chan struct{}
}

func NewStore(sb *sectorbuilder.SectorBuilder) *Store {
	return &Store{
		sb:      sb,
		waiting: map[uint64]chan struct{}{},
		closeCh: make(chan struct{}),
	}
}

func (s *Store) Service() {
	go s.service()
}

func (s *Store) poll() {
	log.Debug("polling for sealed sectors...")

	// get a list of sectors to poll
	s.lk.Lock()
	toPoll := make([]uint64, 0, len(s.waiting))

	for id := range s.waiting {
		toPoll = append(toPoll, id)
	}
	s.lk.Unlock()

	var done []sectorbuilder.SectorSealingStatus

	// check status of each
	for _, sec := range toPoll {
		status, err := s.sb.SealStatus(sec)
		if err != nil {
			log.Errorf("getting seal status: %s", err)
			continue
		}

		if status.SealStatusCode == 0 { // constant pls, zero implies the last step?
			done = append(done, status)
		}
	}

	// send updates
	s.lk.Lock()
	for _, sector := range done {
		watch, ok := s.waiting[sector.SectorID]
		if ok {
			close(watch)
			delete(s.waiting, sector.SectorID)
		}
		for _, c := range s.incoming {
			c <- sector // TODO: ctx!
		}
	}
	s.lk.Unlock()
}

func (s *Store) service() {
	poll := time.Tick(5 * time.Second)

	for {
		select {
		case <-poll:
			s.poll()
		case <-s.closeCh:
			s.lk.Lock()
			for _, c := range s.incoming {
				close(c)
			}
			s.lk.Unlock()
			return
		}
	}
}

func (s *Store) AddPiece(ref string, size uint64, r io.Reader) (sectorID uint64, err error) {
	err = withTemp(r, func(f string) (err error) {
		sectorID, err = s.sb.AddPiece(ref, size, f)
		return err
	})
	if err != nil {
		return 0, err
	}

	s.lk.Lock()
	_, exists := s.waiting[sectorID]
	if !exists { // pieces can share sectors
		s.waiting[sectorID] = make(chan struct{})
	}
	s.lk.Unlock()

	return sectorID, nil
}

func (s *Store) CloseIncoming(c <-chan sectorbuilder.SectorSealingStatus) {
	s.lk.Lock()
	var at = -1
	for i, ch := range s.incoming {
		if ch == c {
			at = i
		}
	}
	if at == -1 {
		s.lk.Unlock()
		return
	}
	if len(s.incoming) > 1 {
		last := len(s.incoming) - 1
		s.incoming[at] = s.incoming[last]
		s.incoming[last] = nil
	}
	s.incoming = s.incoming[:len(s.incoming)-1]
	s.lk.Unlock()
}

func (s *Store) Incoming() <-chan sectorbuilder.SectorSealingStatus {
	ch := make(chan sectorbuilder.SectorSealingStatus, 8)
	s.lk.Lock()
	s.incoming = append(s.incoming, ch)
	s.lk.Unlock()
	return ch
}

func (s *Store) WaitSeal(ctx context.Context, sector uint64) (sectorbuilder.SectorSealingStatus, error) {
	s.lk.Lock()
	watch, ok := s.waiting[sector]
	s.lk.Unlock()
	if ok {
		select {
		case <-watch:
		case <-ctx.Done():
			return sectorbuilder.SectorSealingStatus{}, ctx.Err()
		}
	}

	return s.sb.SealStatus(sector)
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

func withTemp(r io.Reader, cb func(string) error) error {
	f, err := ioutil.TempFile(os.TempDir(), "lotus-temp-")
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	err = cb(f.Name())
	if err != nil {
		if err := os.Remove(f.Name()); err != nil {
			log.Errorf("couldn't remove temp file '%s'", f.Name())
		}
		return err
	}

	return os.Remove(f.Name())
}
