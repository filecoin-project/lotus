package splitstore

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"golang.org/x/xerrors"

	cid "github.com/ipfs/go-cid"

	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/types"
)

// performs an asynchronous health-check on the splitstore; results are appended to
// <splitstore-path>/check.txt
func (s *SplitStore) Check() error {
	s.headChangeMx.Lock()
	defer s.headChangeMx.Unlock()

	// try to take compaction lock and inhibit compaction while the health-check is running
	if !atomic.CompareAndSwapInt32(&s.compacting, 0, 1) {
		return xerrors.Errorf("can't acquire compaction lock; compacting operation in progress")
	}

	if s.compactionIndex == 0 {
		atomic.StoreInt32(&s.compacting, 0)
		return xerrors.Errorf("splitstore hasn't compacted yet; health check is not meaningful")
	}

	// check if we are actually closing first
	if err := s.checkClosing(); err != nil {
		atomic.StoreInt32(&s.compacting, 0)
		return err
	}

	curTs := s.chain.GetHeaviestTipSet()
	go func() {
		defer atomic.StoreInt32(&s.compacting, 0)

		log.Info("checking splitstore health")
		start := time.Now()

		err := s.doCheck(curTs)
		if err != nil {
			log.Errorf("error checking splitstore health: %s", err)
			return
		}

		log.Infow("health check done", "took", time.Since(start))
	}()

	return nil
}

func (s *SplitStore) doCheck(curTs *types.TipSet) error {
	currentEpoch := curTs.Height()
	boundaryEpoch := currentEpoch - CompactionBoundary

	outputPath := filepath.Join(s.path, "check.txt")
	output, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return xerrors.Errorf("error opening check output file %s: %w", outputPath, err)
	}
	defer output.Close() //nolint:errcheck

	write := func(format string, args ...interface{}) {
		_, err := fmt.Fprintf(output, format+"\n", args...)
		if err != nil {
			log.Warnf("error writing check output: %s", err)
		}
	}

	ts, _ := time.Now().MarshalText()
	write("---------------------------------------------")
	write("start check at %s", ts)
	write("current epoch: %d", currentEpoch)
	write("boundary epoch: %d", boundaryEpoch)
	write("compaction index: %d", s.compactionIndex)
	write("--")

	var coldCnt, missingCnt int64

	visitor, err := s.markSetEnv.CreateVisitor("check", 0)
	if err != nil {
		return xerrors.Errorf("error creating visitor: %w", err)
	}
	defer visitor.Close() //nolint

	err = s.walkChain(curTs, boundaryEpoch, boundaryEpoch, visitor,
		func(c cid.Cid) error {
			if isUnitaryObject(c) {
				return errStopWalk
			}

			has, err := s.hot.Has(c)
			if err != nil {
				return xerrors.Errorf("error checking hotstore: %w", err)
			}

			if has {
				return nil
			}

			has, err = s.cold.Has(c)
			if err != nil {
				return xerrors.Errorf("error checking coldstore: %w", err)
			}

			if has {
				coldCnt++
				write("cold object reference: %s", c)
			} else {
				missingCnt++
				write("missing object reference: %s", c)
				return errStopWalk
			}

			return nil
		})

	if err != nil {
		err = xerrors.Errorf("error walking chain: %w", err)
		write("ERROR: %s", err)
		return err
	}

	log.Infow("check done", "cold", coldCnt, "missing", missingCnt)
	write("--")
	write("cold: %d missing: %d", coldCnt, missingCnt)
	write("DONE")

	return nil
}

// provides some basic information about the splitstore
func (s *SplitStore) Info() map[string]interface{} {
	info := make(map[string]interface{})
	info["base epoch"] = s.baseEpoch
	info["warmup epoch"] = s.warmupEpoch
	info["compactions"] = s.compactionIndex

	sizer, ok := s.hot.(bstore.BlockstoreSize)
	if ok {
		size, err := sizer.Size()
		if err != nil {
			log.Warnf("error getting hotstore size: %s", err)
		} else {
			info["hotstore size"] = size
		}
	}

	return info
}
