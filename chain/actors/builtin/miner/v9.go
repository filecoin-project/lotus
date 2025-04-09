package miner

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-bitfield"
	rle "github.com/filecoin-project/go-bitfield/rle"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	builtin9 "github.com/filecoin-project/go-state-types/builtin"
	miner9 "github.com/filecoin-project/go-state-types/builtin/v9/miner"
	adt9 "github.com/filecoin-project/go-state-types/builtin/v9/util/adt"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state9)(nil)

func load9(store adt.Store, root cid.Cid) (State, error) {
	out := state9{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make9(store adt.Store) (State, error) {
	out := state9{store: store}
	out.State = miner9.State{}
	return &out, nil
}

type state9 struct {
	miner9.State
	store adt.Store
}

type deadline9 struct {
	miner9.Deadline
	store adt.Store
}

type partition9 struct {
	miner9.Partition
	store adt.Store
}

func (s *state9) AvailableBalance(bal abi.TokenAmount) (available abi.TokenAmount, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = xerrors.Errorf("failed to get available balance: %w", r)
			available = abi.NewTokenAmount(0)
		}
	}()
	// this panics if the miner doesn't have enough funds to cover their locked pledge
	available, err = s.GetAvailableBalance(bal)
	return available, err
}

func (s *state9) VestedFunds(epoch abi.ChainEpoch) (abi.TokenAmount, error) {
	return s.CheckVestedFunds(s.store, epoch)
}

func (s *state9) LockedFunds() (LockedFunds, error) {
	return LockedFunds{
		VestingFunds:             s.State.LockedFunds,
		InitialPledgeRequirement: s.State.InitialPledge,
		PreCommitDeposits:        s.State.PreCommitDeposits,
	}, nil
}

func (s *state9) FeeDebt() (abi.TokenAmount, error) {
	return s.State.FeeDebt, nil
}

func (s *state9) InitialPledge() (abi.TokenAmount, error) {
	return s.State.InitialPledge, nil
}

func (s *state9) PreCommitDeposits() (abi.TokenAmount, error) {
	return s.State.PreCommitDeposits, nil
}

// Returns nil, nil if sector is not found
func (s *state9) GetSector(num abi.SectorNumber) (*SectorOnChainInfo, error) {
	info, ok, err := s.State.GetSector(s.store, num)
	if !ok || err != nil {
		return nil, err
	}

	ret := fromV9SectorOnChainInfo(*info)
	return &ret, nil
}

func (s *state9) FindSector(num abi.SectorNumber) (*SectorLocation, error) {
	dlIdx, partIdx, err := s.State.FindSector(s.store, num)
	if err != nil {
		return nil, err
	}
	return &SectorLocation{
		Deadline:  dlIdx,
		Partition: partIdx,
	}, nil
}

func (s *state9) NumLiveSectors() (uint64, error) {
	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return 0, err
	}
	var total uint64
	if err := dls.ForEach(s.store, func(dlIdx uint64, dl *miner9.Deadline) error {
		total += dl.LiveSectors
		return nil
	}); err != nil {
		return 0, err
	}
	return total, nil
}

// GetSectorExpiration returns the effective expiration of the given sector.
//
// If the sector does not expire early, the Early expiration field is 0.
func (s *state9) GetSectorExpiration(num abi.SectorNumber) (*SectorExpiration, error) {
	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return nil, err
	}
	// NOTE: this can be optimized significantly.
	// 1. If the sector is non-faulty, it will expire on-time (can be
	// learned from the sector info).
	// 2. If it's faulty, it will expire early within the first 42 entries
	// of the expiration queue.

	stopErr := errors.New("stop")
	out := SectorExpiration{}
	err = dls.ForEach(s.store, func(dlIdx uint64, dl *miner9.Deadline) error {
		partitions, err := dl.PartitionsArray(s.store)
		if err != nil {
			return err
		}
		quant := s.State.QuantSpecForDeadline(dlIdx)
		var part miner9.Partition
		return partitions.ForEach(&part, func(partIdx int64) error {
			if found, err := part.Sectors.IsSet(uint64(num)); err != nil {
				return err
			} else if !found {
				return nil
			}
			if found, err := part.Terminated.IsSet(uint64(num)); err != nil {
				return err
			} else if found {
				// already terminated
				return stopErr
			}

			q, err := miner9.LoadExpirationQueue(s.store, part.ExpirationsEpochs, quant, miner9.PartitionExpirationAmtBitwidth)
			if err != nil {
				return err
			}
			var exp miner9.ExpirationSet
			return q.ForEach(&exp, func(epoch int64) error {
				if early, err := exp.EarlySectors.IsSet(uint64(num)); err != nil {
					return err
				} else if early {
					out.Early = abi.ChainEpoch(epoch)
					return nil
				}
				if onTime, err := exp.OnTimeSectors.IsSet(uint64(num)); err != nil {
					return err
				} else if onTime {
					out.OnTime = abi.ChainEpoch(epoch)
					return stopErr
				}
				return nil
			})
		})
	})
	if err == stopErr {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	if out.Early == 0 && out.OnTime == 0 {
		return nil, xerrors.Errorf("failed to find sector %d", num)
	}
	return &out, nil
}

func (s *state9) GetPrecommittedSector(num abi.SectorNumber) (*SectorPreCommitOnChainInfo, error) {
	info, ok, err := s.State.GetPrecommittedSector(s.store, num)
	if !ok || err != nil {
		return nil, err
	}

	ret := fromV9SectorPreCommitOnChainInfo(*info)

	return &ret, nil
}

func (s *state9) ForEachPrecommittedSector(cb func(SectorPreCommitOnChainInfo) error) error {
	precommitted, err := adt9.AsMap(s.store, s.State.PreCommittedSectors, builtin9.DefaultHamtBitwidth)
	if err != nil {
		return err
	}

	var info miner9.SectorPreCommitOnChainInfo
	if err := precommitted.ForEach(&info, func(_ string) error {
		return cb(fromV9SectorPreCommitOnChainInfo(info))
	}); err != nil {
		return err
	}

	return nil
}

func (s *state9) LoadSectors(snos *bitfield.BitField) ([]*SectorOnChainInfo, error) {
	sectors, err := miner9.LoadSectors(s.store, s.State.Sectors)
	if err != nil {
		return nil, err
	}

	// If no sector numbers are specified, load all.
	if snos == nil {
		infos := make([]*SectorOnChainInfo, 0, sectors.Length())
		var info9 miner9.SectorOnChainInfo
		if err := sectors.ForEach(&info9, func(_ int64) error {
			info := fromV9SectorOnChainInfo(info9)
			infos = append(infos, &info)
			return nil
		}); err != nil {
			return nil, err
		}
		return infos, nil
	}

	// Otherwise, load selected.
	infos9, err := sectors.Load(*snos)
	if err != nil {
		return nil, err
	}
	infos := make([]*SectorOnChainInfo, len(infos9))
	for i, info9 := range infos9 {
		info := fromV9SectorOnChainInfo(*info9)
		infos[i] = &info
	}
	return infos, nil
}

func (s *state9) loadAllocatedSectorNumbers() (bitfield.BitField, error) {
	var allocatedSectors bitfield.BitField
	err := s.store.Get(s.store.Context(), s.State.AllocatedSectors, &allocatedSectors)
	return allocatedSectors, err
}

func (s *state9) IsAllocated(num abi.SectorNumber) (bool, error) {
	allocatedSectors, err := s.loadAllocatedSectorNumbers()
	if err != nil {
		return false, err
	}

	return allocatedSectors.IsSet(uint64(num))
}

func (s *state9) GetProvingPeriodStart() (abi.ChainEpoch, error) {
	return s.State.ProvingPeriodStart, nil
}

func (s *state9) UnallocatedSectorNumbers(count int) ([]abi.SectorNumber, error) {
	allocatedSectors, err := s.loadAllocatedSectorNumbers()
	if err != nil {
		return nil, err
	}

	allocatedRuns, err := allocatedSectors.RunIterator()
	if err != nil {
		return nil, err
	}

	unallocatedRuns, err := rle.Subtract(
		&rle.RunSliceIterator{Runs: []rle.Run{{Val: true, Len: abi.MaxSectorNumber}}},
		allocatedRuns,
	)
	if err != nil {
		return nil, err
	}

	iter, err := rle.BitsFromRuns(unallocatedRuns)
	if err != nil {
		return nil, err
	}

	sectors := make([]abi.SectorNumber, 0, count)
	for iter.HasNext() && len(sectors) < count {
		nextNo, err := iter.Next()
		if err != nil {
			return nil, err
		}
		sectors = append(sectors, abi.SectorNumber(nextNo))
	}

	return sectors, nil
}

func (s *state9) GetAllocatedSectors() (*bitfield.BitField, error) {
	var allocatedSectors bitfield.BitField
	if err := s.store.Get(s.store.Context(), s.State.AllocatedSectors, &allocatedSectors); err != nil {
		return nil, err
	}

	return &allocatedSectors, nil
}

func (s *state9) LoadDeadline(idx uint64) (Deadline, error) {
	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return nil, err
	}
	dl, err := dls.LoadDeadline(s.store, idx)
	if err != nil {
		return nil, err
	}
	return &deadline9{*dl, s.store}, nil
}

func (s *state9) ForEachDeadline(cb func(uint64, Deadline) error) error {
	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return err
	}
	return dls.ForEach(s.store, func(i uint64, dl *miner9.Deadline) error {
		return cb(i, &deadline9{*dl, s.store})
	})
}

func (s *state9) NumDeadlines() (uint64, error) {
	return miner9.WPoStPeriodDeadlines, nil
}

func (s *state9) DeadlinesChanged(other State) (bool, error) {
	other9, ok := other.(*state9)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}

	return !s.State.Deadlines.Equals(other9.Deadlines), nil
}

func (s *state9) MinerInfoChanged(other State) (bool, error) {
	other0, ok := other.(*state9)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}
	return !s.State.Info.Equals(other0.State.Info), nil
}

func (s *state9) Info() (MinerInfo, error) {
	info, err := s.State.GetInfo(s.store)
	if err != nil {
		return MinerInfo{}, err
	}

	mi := MinerInfo{
		Owner:            info.Owner,
		Worker:           info.Worker,
		ControlAddresses: info.ControlAddresses,

		PendingWorkerKey: (*WorkerKeyChange)(info.PendingWorkerKey),

		PeerId:                     info.PeerId,
		Multiaddrs:                 info.Multiaddrs,
		WindowPoStProofType:        info.WindowPoStProofType,
		SectorSize:                 info.SectorSize,
		WindowPoStPartitionSectors: info.WindowPoStPartitionSectors,
		ConsensusFaultElapsed:      info.ConsensusFaultElapsed,

		Beneficiary:            info.Beneficiary,
		BeneficiaryTerm:        BeneficiaryTerm(info.BeneficiaryTerm),
		PendingBeneficiaryTerm: (*PendingBeneficiaryChange)(info.PendingBeneficiaryTerm),
	}

	return mi, nil
}

func (s *state9) DeadlineInfo(epoch abi.ChainEpoch) (*dline.Info, error) {
	return s.State.RecordedDeadlineInfo(epoch), nil
}

func (s *state9) DeadlineCronActive() (bool, error) {
	return s.State.DeadlineCronActive, nil
}

func (s *state9) sectors() (adt.Array, error) {
	return adt9.AsArray(s.store, s.Sectors, miner9.SectorsAmtBitwidth)
}

func (s *state9) decodeSectorOnChainInfo(val *cbg.Deferred) (SectorOnChainInfo, error) {
	var si miner9.SectorOnChainInfo
	err := si.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return SectorOnChainInfo{}, err
	}

	return fromV9SectorOnChainInfo(si), nil
}

func (s *state9) precommits() (adt.Map, error) {
	return adt9.AsMap(s.store, s.PreCommittedSectors, builtin9.DefaultHamtBitwidth)
}

func (s *state9) decodeSectorPreCommitOnChainInfo(val *cbg.Deferred) (SectorPreCommitOnChainInfo, error) {
	var sp miner9.SectorPreCommitOnChainInfo
	err := sp.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return SectorPreCommitOnChainInfo{}, err
	}

	return fromV9SectorPreCommitOnChainInfo(sp), nil
}

func (s *state9) EraseAllUnproven() error {

	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return err
	}

	err = dls.ForEach(s.store, func(dindx uint64, dl *miner9.Deadline) error {
		ps, err := dl.PartitionsArray(s.store)
		if err != nil {
			return err
		}

		var part miner9.Partition
		err = ps.ForEach(&part, func(pindx int64) error {
			_ = part.ActivateUnproven()
			err = ps.Set(uint64(pindx), &part)
			return nil
		})

		if err != nil {
			return err
		}

		dl.Partitions, err = ps.Root()
		if err != nil {
			return err
		}

		return dls.UpdateDeadline(s.store, dindx, dl)
	})
	if err != nil {
		return err
	}

	return s.State.SaveDeadlines(s.store, dls)

}

func (d *deadline9) LoadPartition(idx uint64) (Partition, error) {
	p, err := d.Deadline.LoadPartition(d.store, idx)
	if err != nil {
		return nil, err
	}
	return &partition9{*p, d.store}, nil
}

func (d *deadline9) ForEachPartition(cb func(uint64, Partition) error) error {
	ps, err := d.Deadline.PartitionsArray(d.store)
	if err != nil {
		return err
	}
	var part miner9.Partition
	return ps.ForEach(&part, func(i int64) error {
		return cb(uint64(i), &partition9{part, d.store})
	})
}

func (d *deadline9) PartitionsChanged(other Deadline) (bool, error) {
	other9, ok := other.(*deadline9)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}

	return !d.Deadline.Partitions.Equals(other9.Deadline.Partitions), nil
}

func (d *deadline9) PartitionsPoSted() (bitfield.BitField, error) {
	return d.Deadline.PartitionsPoSted, nil
}

func (d *deadline9) DisputableProofCount() (uint64, error) {

	ops, err := d.OptimisticProofsSnapshotArray(d.store)
	if err != nil {
		return 0, err
	}

	return ops.Length(), nil

}

func (d *deadline9) DailyFee() (abi.TokenAmount, error) {
	return big.Zero(), nil
}

func (p *partition9) AllSectors() (bitfield.BitField, error) {
	return p.Partition.Sectors, nil
}

func (p *partition9) FaultySectors() (bitfield.BitField, error) {
	return p.Partition.Faults, nil
}

func (p *partition9) RecoveringSectors() (bitfield.BitField, error) {
	return p.Partition.Recoveries, nil
}

func (p *partition9) UnprovenSectors() (bitfield.BitField, error) {
	return p.Partition.Unproven, nil
}

func fromV9SectorOnChainInfo(v9 miner9.SectorOnChainInfo) SectorOnChainInfo {
	info := SectorOnChainInfo{
		SectorNumber:          v9.SectorNumber,
		SealProof:             v9.SealProof,
		SealedCID:             v9.SealedCID,
		DeprecatedDealIDs:     v9.DealIDs,
		Activation:            v9.Activation,
		Expiration:            v9.Expiration,
		DealWeight:            v9.DealWeight,
		VerifiedDealWeight:    v9.VerifiedDealWeight,
		InitialPledge:         v9.InitialPledge,
		ExpectedDayReward:     &v9.ExpectedDayReward,
		ExpectedStoragePledge: &v9.ExpectedStoragePledge,
		SectorKeyCID:          v9.SectorKeyCID,
		DailyFee:              big.Zero(),
	}
	return info
}

func fromV9SectorPreCommitOnChainInfo(v9 miner9.SectorPreCommitOnChainInfo) SectorPreCommitOnChainInfo {
	ret := SectorPreCommitOnChainInfo{
		Info: SectorPreCommitInfo{
			SealProof:     v9.Info.SealProof,
			SectorNumber:  v9.Info.SectorNumber,
			SealedCID:     v9.Info.SealedCID,
			SealRandEpoch: v9.Info.SealRandEpoch,
			DealIDs:       v9.Info.DealIDs,
			Expiration:    v9.Info.Expiration,
			UnsealedCid:   nil,
		},
		PreCommitDeposit: v9.PreCommitDeposit,
		PreCommitEpoch:   v9.PreCommitEpoch,
	}

	ret.Info.UnsealedCid = v9.Info.UnsealedCid

	return ret
}

func (s *state9) GetState() interface{} {
	return &s.State
}

func (s *state9) ActorKey() string {
	return manifest.MinerKey
}

func (s *state9) ActorVersion() actorstypes.Version {
	return actorstypes.Version9
}

func (s *state9) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
