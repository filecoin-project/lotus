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
	builtin14 "github.com/filecoin-project/go-state-types/builtin"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	adt14 "github.com/filecoin-project/go-state-types/builtin/v14/util/adt"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state14)(nil)

func load14(store adt.Store, root cid.Cid) (State, error) {
	out := state14{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make14(store adt.Store) (State, error) {
	out := state14{store: store}
	out.State = miner14.State{}
	return &out, nil
}

type state14 struct {
	miner14.State
	store adt.Store
}

type deadline14 struct {
	miner14.Deadline
	store adt.Store
}

type partition14 struct {
	miner14.Partition
	store adt.Store
}

func (s *state14) AvailableBalance(bal abi.TokenAmount) (available abi.TokenAmount, err error) {
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

func (s *state14) VestedFunds(epoch abi.ChainEpoch) (abi.TokenAmount, error) {
	return s.CheckVestedFunds(s.store, epoch)
}

func (s *state14) LockedFunds() (LockedFunds, error) {
	return LockedFunds{
		VestingFunds:             s.State.LockedFunds,
		InitialPledgeRequirement: s.State.InitialPledge,
		PreCommitDeposits:        s.State.PreCommitDeposits,
	}, nil
}

func (s *state14) FeeDebt() (abi.TokenAmount, error) {
	return s.State.FeeDebt, nil
}

func (s *state14) InitialPledge() (abi.TokenAmount, error) {
	return s.State.InitialPledge, nil
}

func (s *state14) PreCommitDeposits() (abi.TokenAmount, error) {
	return s.State.PreCommitDeposits, nil
}

// Returns nil, nil if sector is not found
func (s *state14) GetSector(num abi.SectorNumber) (*SectorOnChainInfo, error) {
	info, ok, err := s.State.GetSector(s.store, num)
	if !ok || err != nil {
		return nil, err
	}

	ret := fromV14SectorOnChainInfo(*info)
	return &ret, nil
}

func (s *state14) FindSector(num abi.SectorNumber) (*SectorLocation, error) {
	dlIdx, partIdx, err := s.State.FindSector(s.store, num)
	if err != nil {
		return nil, err
	}
	return &SectorLocation{
		Deadline:  dlIdx,
		Partition: partIdx,
	}, nil
}

func (s *state14) NumLiveSectors() (uint64, error) {
	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return 0, err
	}
	var total uint64
	if err := dls.ForEach(s.store, func(dlIdx uint64, dl *miner14.Deadline) error {
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
func (s *state14) GetSectorExpiration(num abi.SectorNumber) (*SectorExpiration, error) {
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
	err = dls.ForEach(s.store, func(dlIdx uint64, dl *miner14.Deadline) error {
		partitions, err := dl.PartitionsArray(s.store)
		if err != nil {
			return err
		}
		quant := s.State.QuantSpecForDeadline(dlIdx)
		var part miner14.Partition
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

			q, err := miner14.LoadExpirationQueue(s.store, part.ExpirationsEpochs, quant, miner14.PartitionExpirationAmtBitwidth)
			if err != nil {
				return err
			}
			var exp miner14.ExpirationSet
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

func (s *state14) GetPrecommittedSector(num abi.SectorNumber) (*SectorPreCommitOnChainInfo, error) {
	info, ok, err := s.State.GetPrecommittedSector(s.store, num)
	if !ok || err != nil {
		return nil, err
	}

	ret := fromV14SectorPreCommitOnChainInfo(*info)

	return &ret, nil
}

func (s *state14) ForEachPrecommittedSector(cb func(SectorPreCommitOnChainInfo) error) error {
	precommitted, err := adt14.AsMap(s.store, s.State.PreCommittedSectors, builtin14.DefaultHamtBitwidth)
	if err != nil {
		return err
	}

	var info miner14.SectorPreCommitOnChainInfo
	if err := precommitted.ForEach(&info, func(_ string) error {
		return cb(fromV14SectorPreCommitOnChainInfo(info))
	}); err != nil {
		return err
	}

	return nil
}

func (s *state14) LoadSectors(snos *bitfield.BitField) ([]*SectorOnChainInfo, error) {
	sectors, err := miner14.LoadSectors(s.store, s.State.Sectors)
	if err != nil {
		return nil, err
	}

	// If no sector numbers are specified, load all.
	if snos == nil {
		infos := make([]*SectorOnChainInfo, 0, sectors.Length())
		var info14 miner14.SectorOnChainInfo
		if err := sectors.ForEach(&info14, func(_ int64) error {
			info := fromV14SectorOnChainInfo(info14)
			infos = append(infos, &info)
			return nil
		}); err != nil {
			return nil, err
		}
		return infos, nil
	}

	// Otherwise, load selected.
	infos14, err := sectors.Load(*snos)
	if err != nil {
		return nil, err
	}
	infos := make([]*SectorOnChainInfo, len(infos14))
	for i, info14 := range infos14 {
		info := fromV14SectorOnChainInfo(*info14)
		infos[i] = &info
	}
	return infos, nil
}

func (s *state14) loadAllocatedSectorNumbers() (bitfield.BitField, error) {
	var allocatedSectors bitfield.BitField
	err := s.store.Get(s.store.Context(), s.State.AllocatedSectors, &allocatedSectors)
	return allocatedSectors, err
}

func (s *state14) IsAllocated(num abi.SectorNumber) (bool, error) {
	allocatedSectors, err := s.loadAllocatedSectorNumbers()
	if err != nil {
		return false, err
	}

	return allocatedSectors.IsSet(uint64(num))
}

func (s *state14) GetProvingPeriodStart() (abi.ChainEpoch, error) {
	return s.State.ProvingPeriodStart, nil
}

func (s *state14) UnallocatedSectorNumbers(count int) ([]abi.SectorNumber, error) {
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

func (s *state14) GetAllocatedSectors() (*bitfield.BitField, error) {
	var allocatedSectors bitfield.BitField
	if err := s.store.Get(s.store.Context(), s.State.AllocatedSectors, &allocatedSectors); err != nil {
		return nil, err
	}

	return &allocatedSectors, nil
}

func (s *state14) LoadDeadline(idx uint64) (Deadline, error) {
	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return nil, err
	}
	dl, err := dls.LoadDeadline(s.store, idx)
	if err != nil {
		return nil, err
	}
	return &deadline14{*dl, s.store}, nil
}

func (s *state14) ForEachDeadline(cb func(uint64, Deadline) error) error {
	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return err
	}
	return dls.ForEach(s.store, func(i uint64, dl *miner14.Deadline) error {
		return cb(i, &deadline14{*dl, s.store})
	})
}

func (s *state14) NumDeadlines() (uint64, error) {
	return miner14.WPoStPeriodDeadlines, nil
}

func (s *state14) DeadlinesChanged(other State) (bool, error) {
	other14, ok := other.(*state14)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}

	return !s.State.Deadlines.Equals(other14.Deadlines), nil
}

func (s *state14) MinerInfoChanged(other State) (bool, error) {
	other0, ok := other.(*state14)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}
	return !s.State.Info.Equals(other0.State.Info), nil
}

func (s *state14) Info() (MinerInfo, error) {
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

func (s *state14) DeadlineInfo(epoch abi.ChainEpoch) (*dline.Info, error) {
	return s.State.RecordedDeadlineInfo(epoch), nil
}

func (s *state14) DeadlineCronActive() (bool, error) {
	return s.State.DeadlineCronActive, nil
}

func (s *state14) sectors() (adt.Array, error) {
	return adt14.AsArray(s.store, s.Sectors, miner14.SectorsAmtBitwidth)
}

func (s *state14) decodeSectorOnChainInfo(val *cbg.Deferred) (SectorOnChainInfo, error) {
	var si miner14.SectorOnChainInfo
	err := si.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return SectorOnChainInfo{}, err
	}

	return fromV14SectorOnChainInfo(si), nil
}

func (s *state14) precommits() (adt.Map, error) {
	return adt14.AsMap(s.store, s.PreCommittedSectors, builtin14.DefaultHamtBitwidth)
}

func (s *state14) decodeSectorPreCommitOnChainInfo(val *cbg.Deferred) (SectorPreCommitOnChainInfo, error) {
	var sp miner14.SectorPreCommitOnChainInfo
	err := sp.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return SectorPreCommitOnChainInfo{}, err
	}

	return fromV14SectorPreCommitOnChainInfo(sp), nil
}

func (s *state14) EraseAllUnproven() error {

	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return err
	}

	err = dls.ForEach(s.store, func(dindx uint64, dl *miner14.Deadline) error {
		ps, err := dl.PartitionsArray(s.store)
		if err != nil {
			return err
		}

		var part miner14.Partition
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

func (d *deadline14) LoadPartition(idx uint64) (Partition, error) {
	p, err := d.Deadline.LoadPartition(d.store, idx)
	if err != nil {
		return nil, err
	}
	return &partition14{*p, d.store}, nil
}

func (d *deadline14) ForEachPartition(cb func(uint64, Partition) error) error {
	ps, err := d.Deadline.PartitionsArray(d.store)
	if err != nil {
		return err
	}
	var part miner14.Partition
	return ps.ForEach(&part, func(i int64) error {
		return cb(uint64(i), &partition14{part, d.store})
	})
}

func (d *deadline14) PartitionsChanged(other Deadline) (bool, error) {
	other14, ok := other.(*deadline14)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}

	return !d.Deadline.Partitions.Equals(other14.Deadline.Partitions), nil
}

func (d *deadline14) PartitionsPoSted() (bitfield.BitField, error) {
	return d.Deadline.PartitionsPoSted, nil
}

func (d *deadline14) DisputableProofCount() (uint64, error) {

	ops, err := d.OptimisticProofsSnapshotArray(d.store)
	if err != nil {
		return 0, err
	}

	return ops.Length(), nil

}

func (d *deadline14) DailyFee() (abi.TokenAmount, error) {
	return big.Zero(), nil
}

func (p *partition14) AllSectors() (bitfield.BitField, error) {
	return p.Partition.Sectors, nil
}

func (p *partition14) FaultySectors() (bitfield.BitField, error) {
	return p.Partition.Faults, nil
}

func (p *partition14) RecoveringSectors() (bitfield.BitField, error) {
	return p.Partition.Recoveries, nil
}

func (p *partition14) UnprovenSectors() (bitfield.BitField, error) {
	return p.Partition.Unproven, nil
}

func fromV14SectorOnChainInfo(v14 miner14.SectorOnChainInfo) SectorOnChainInfo {
	info := SectorOnChainInfo{
		SectorNumber:          v14.SectorNumber,
		SealProof:             v14.SealProof,
		SealedCID:             v14.SealedCID,
		DeprecatedDealIDs:     v14.DealIDs,
		Activation:            v14.Activation,
		Expiration:            v14.Expiration,
		DealWeight:            v14.DealWeight,
		VerifiedDealWeight:    v14.VerifiedDealWeight,
		InitialPledge:         v14.InitialPledge,
		ExpectedDayReward:     &v14.ExpectedDayReward,
		ExpectedStoragePledge: &v14.ExpectedStoragePledge,
		SectorKeyCID:          v14.SectorKeyCID,
		PowerBaseEpoch:        v14.PowerBaseEpoch,
		ReplacedDayReward:     &v14.ReplacedDayReward,
		Flags:                 SectorOnChainInfoFlags(v14.Flags),
		DailyFee:              big.Zero(),
	}
	return info
}

func fromV14SectorPreCommitOnChainInfo(v14 miner14.SectorPreCommitOnChainInfo) SectorPreCommitOnChainInfo {
	ret := SectorPreCommitOnChainInfo{
		Info: SectorPreCommitInfo{
			SealProof:     v14.Info.SealProof,
			SectorNumber:  v14.Info.SectorNumber,
			SealedCID:     v14.Info.SealedCID,
			SealRandEpoch: v14.Info.SealRandEpoch,
			DealIDs:       v14.Info.DealIDs,
			Expiration:    v14.Info.Expiration,
			UnsealedCid:   nil,
		},
		PreCommitDeposit: v14.PreCommitDeposit,
		PreCommitEpoch:   v14.PreCommitEpoch,
	}

	ret.Info.UnsealedCid = v14.Info.UnsealedCid

	return ret
}

func (s *state14) GetState() interface{} {
	return &s.State
}

func (s *state14) ActorKey() string {
	return manifest.MinerKey
}

func (s *state14) ActorVersion() actorstypes.Version {
	return actorstypes.Version14
}

func (s *state14) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
