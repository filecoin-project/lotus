package sectorbuilder

import (
	"io/ioutil"
	"os"
	"path/filepath"

	sectorbuilder "github.com/filecoin-project/filecoin-ffi"
	"golang.org/x/xerrors"
)

type Fault struct {
	SectorID uint64

	Err error
}

func (sb *SectorBuilder) Scrub(sectorSet sectorbuilder.SortedPublicSectorInfo) []*Fault {
	var faults []*Fault

	for _, sector := range sectorSet.Values() {
		err := sb.checkSector(sector.SectorID)
		if err != nil {
			faults = append(faults, &Fault{SectorID: sector.SectorID, Err: err})
		}
	}

	return faults
}

func (sb *SectorBuilder) checkSector(sectorID uint64) error {
	cache, err := sb.sectorCacheDir(sectorID)
	if err != nil {
		return xerrors.Errorf("getting sector cache dir: %w", err)
	}

	if err := assertFile(filepath.Join(cache, "p_aux"), 96, 96); err != nil {
		return err
	}
	if err := assertFile(filepath.Join(cache, "sc-01-data-tree-r-last.dat"), (2*sb.ssize)-32, (2*sb.ssize)-32); err != nil {
		return err
	}

	// TODO: better validate this
	if err := assertFile(filepath.Join(cache, "t_aux"), 100, 32000); err != nil { // TODO: what should this actually be?
		return err
	}

	dent, err := ioutil.ReadDir(cache)
	if err != nil {
		return xerrors.Errorf("reading cache dir %s", cache)
	}
	if len(dent) != 3 {
		return xerrors.Errorf("found %d files in %s, expected 3", len(dent), cache)
	}

	sealed, err := sb.SealedSectorPath(sectorID)
	if err != nil {
		return xerrors.Errorf("getting sealed sector path: %w", err)
	}

	if err := assertFile(filepath.Join(sealed), sb.ssize, sb.ssize); err != nil {
		return err
	}

	return nil
}

func assertFile(path string, minSz uint64, maxSz uint64) error {
	st, err := os.Stat(path)
	if err != nil {
		return xerrors.Errorf("stat %s: %w", path, err)
	}

	if st.IsDir() {
		return xerrors.Errorf("expected %s to be a regular file", path)
	}

	if uint64(st.Size()) < minSz || uint64(st.Size()) > maxSz {
		return xerrors.Errorf("%s wasn't within size bounds, expected %d < f < %d, got %d", minSz, maxSz, st.Size())
	}

	return nil
}
