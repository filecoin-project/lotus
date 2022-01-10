package miner

import (
	"context"
	"crypto/rand"
	"math"
	"os"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"

	proof2 "github.com/filecoin-project/specs-actors/v2/actors/runtime/proof"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
)

func (m *Miner) winPoStWarmup(ctx context.Context) error {
	deadlines, err := m.api.StateMinerDeadlines(ctx, m.address, types.EmptyTSK)
	if err != nil {
		return xerrors.Errorf("getting deadlines: %w", err)
	}

	var sector abi.SectorNumber = math.MaxUint64

out:
	for dlIdx := range deadlines {
		partitions, err := m.api.StateMinerPartitions(ctx, m.address, uint64(dlIdx), types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting partitions for deadline %d: %w", dlIdx, err)
		}

		for _, partition := range partitions {
			b, err := partition.ActiveSectors.First()
			if err == bitfield.ErrNoBitsSet {
				continue
			}
			if err != nil {
				return err
			}

			sector = abi.SectorNumber(b)
			break out
		}
	}

	if sector == math.MaxUint64 {
	out2:
		for dlIdx := range deadlines {
			partitions, err := m.api.StateMinerPartitions(ctx, m.address, uint64(dlIdx), types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("getting partitions for deadline %d: %w", dlIdx, err)
			}

			for _, partition := range partitions {
				b, err := partition.LiveSectors.First()
				if err == bitfield.ErrNoBitsSet {
					continue
				}
				if err != nil {
					return err
				}

				sector = abi.SectorNumber(b)
				break out2
			}
		}

		if sector == math.MaxUint64 {
			log.Info("skipping winning PoSt warmup, no sectors")
			return nil
		}
	}

	log.Infow("starting winning PoSt warmup", "sector", sector)
	start := time.Now()

	var r abi.PoStRandomness = make([]byte, abi.RandomnessLength)
	_, _ = rand.Read(r)

	si, err := m.api.StateSectorGetInfo(ctx, m.address, sector, types.EmptyTSK)
	if err != nil {
		return xerrors.Errorf("getting sector info: %w", err)
	}

	sis := []proof2.SectorInfo{
		{
			SealProof:    si.SealProof,
			SectorNumber: sector,
			SealedCID:    si.SealedCID,
		},
	}

	wp, err := m.epp.ComputeProof(ctx, sis, r)
	if err != nil {
		return xerrors.Errorf("failed to compute proof: %w", err)
	}

	log.Infow("winning PoSt warmup successful", "took", time.Now().Sub(start))

	aid, err := address.IDFromAddress(m.address)
	if err != nil {
		return xerrors.Errorf("getting id from address: %w", err)
	}

	ok, err := ffiwrapper.ProofVerifier.VerifyWinningPoSt(ctx, proof2.WinningPoStVerifyInfo{
		Randomness:        r,
		Proofs:            wp,
		ChallengedSectors: sis,
		Prover:            abi.ActorID(aid),
	})
	if err != nil {
		return xerrors.Errorf("verifying warmup winning post: %w", err)
	}
	if !ok {
		log.Error("winning PoSt warmup failed to verify")
	} else {
		log.Info("winning PoSt warmup validated successfully")
	}

	if os.Getenv("WARMUP_WINDOW_POST") == "1" {
		wp, sk, err := m.epp.GenerateWindowPoSt(ctx, sis, r)
		if err != nil {
			return xerrors.Errorf("computing window post: %w", err)
		}
		if len(sk) > 0 {
			log.Errorf("skipped sectors in winning post: %+v", sk)
		}

		log.Infow("window PoSt warmup successful", "took", time.Now().Sub(start))

		ok, err := ffiwrapper.ProofVerifier.VerifyWindowPoSt(ctx, proof2.WindowPoStVerifyInfo{
			Randomness:        r,
			Proofs:            wp,
			ChallengedSectors: sis,
			Prover:            abi.ActorID(aid),
		})
		if err != nil {
			return xerrors.Errorf("verifying warmup window post: %w", err)
		}
		if !ok {
			log.Error("window PoSt warmup failed to verify")
		} else {
			log.Info("window PoSt warmup validated successfully")
		}
	}

	return nil
}

func (m *Miner) doWinPoStWarmup(ctx context.Context) {
	err := m.winPoStWarmup(ctx)
	if err != nil {
		log.Errorw("winning PoSt warmup failed", "error", err)
	}
}
