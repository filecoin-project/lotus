package stores

import (
	"context"
	"sync"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi"
)

type sectorLock struct {
	lk sync.Mutex
	notif *ctxCond

	r [FileTypes]uint
	w SectorFileType

	refs uint // access with indexLocks.lk
}

func (l *sectorLock) canLock(read SectorFileType, write SectorFileType) bool {
	for i, b := range write.All() {
		if b && l.r[i] > 0 {
			return false
		}
	}

	return l.w & (read | write) == 0
}

func (l *sectorLock) tryLock(read SectorFileType, write SectorFileType) bool {
	if !l.canLock(read, write) {
		return false
	}

	for i, set := range read.All() {
		if set {
			l.r[i]++
		}
	}

	l.w |= write

	return true
}

func (l *sectorLock) lock(ctx context.Context, read SectorFileType, write SectorFileType) error {
	l.lk.Lock()
	defer l.lk.Unlock()

	for {
		if l.tryLock(read, write) {
			return nil
		}

		if err := l.notif.Wait(ctx); err != nil {
			return err
		}
	}
}

func (l *sectorLock) unlock(read SectorFileType, write SectorFileType) {
	l.lk.Lock()
	defer l.lk.Unlock()

	for i, set := range read.All() {
		if set {
			l.r[i]--
		}
	}

	l.w &= ^write

	l.notif.Broadcast()
}

type indexLocks struct {
	lk sync.Mutex

	locks map[abi.SectorID]*sectorLock
}

func (i *indexLocks) StorageLock(ctx context.Context, sector abi.SectorID, read SectorFileType, write SectorFileType) error {
	if read | write == 0 {
		return nil
	}

	if read | write > (1 << FileTypes) - 1 {
		return xerrors.Errorf("unknown file types specified")
	}

	i.lk.Lock()
	slk, ok := i.locks[sector]
	if !ok {
		slk = &sectorLock{}
		slk.notif = newCtxCond(&slk.lk)
		i.locks[sector] = slk
	}

	slk.refs++

	i.lk.Unlock()

	if err := slk.lock(ctx, read, write); err != nil {
		return err
	}

	go func() {
		// TODO: we can avoid this goroutine with a bit of creativity and reflect

		<-ctx.Done()
		i.lk.Lock()

		slk.unlock(read, write)
		slk.refs--

		if slk.refs == 0 {
			delete(i.locks, sector)
		}

		i.lk.Unlock()
	}()

	return nil
}

