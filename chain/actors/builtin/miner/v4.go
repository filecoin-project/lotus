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
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/manifest"
	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"
	miner4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/miner"
	adt4 "github.com/filecoin-project/specs-actors/v4/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state4)(nil)

func load4(store adt.Store, root cid.Cid) (State, error) {
	out := state4{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make4(store adt.Store) (State, error) {
	out := state4{store: store}
	out.State = miner4.State{}
	return &out, nil
}

type state4 struct {
	miner4.State
	store adt.Store
}

type deadline4 struct {
	miner4.Deadline
	store adt.Store
}

type partition4 struct {
	miner4.Partition
	store adt.Store
}

func (s *state4) AvailableBalance(bal abi.TokenAmount) (available abi.TokenAmount, err error) {
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

func (s *state4) VestedFunds(epoch abi.ChainEpoch) (abi.TokenAmount, error) {
	return s.CheckVestedFunds(s.store, epoch)
}

func (s *state4) LockedFunds() (LockedFunds, error) {
	return LockedFunds{
		VestingFunds:             s.State.LockedFunds,
		InitialPledgeRequirement: s.State.InitialPledge,
		PreCommitDeposits:        s.State.PreCommitDeposits,
	}, nil
}

func (s *state4) FeeDebt() (abi.TokenAmount, error) {
	return s.State.FeeDebt, nil
}

func (s *state4) InitialPledge() (abi.TokenAmount, error) {
	return s.State.InitialPledge, nil
}

func (s *state4) PreCommitDeposits() (abi.TokenAmount, error) {
	return s.State.PreCommitDeposits, nil
}

// Returns nil, nil if sector is not found
func (s *state4) GetSector(num abi.SectorNumber) (*SectorOnChainInfo, error) {
	info, ok, err := s.State.GetSector(s.store, num)
	if !ok || err != nil {
		return nil, err
	}

	ret := fromV4SectorOnChainInfo(*info)
	return &ret, nil
}

func (s *state4) FindSector(num abi.SectorNumber) (*SectorLocation, error) {
	dlIdx, partIdx, err := s.State.FindSector(s.store, num)
	if err != nil {
		return nil, err
	}
	return &SectorLocation{
		Deadline:  dlIdx,
		Partition: partIdx,
	}, nil
}

func (s *state4) NumLiveSectors() (uint64, error) {
	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return 0, err
	}
	var total uint64
	if err := dls.ForEach(s.store, func(dlIdx uint64, dl *miner4.Deadline) error {
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
func (s *state4) GetSectorExpiration(num abi.SectorNumber) (*SectorExpiration, error) {
	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return nil, err
	}
	// NOTE: this can be optimized significantly.
	// 1. If the sector is non-faulty, it will either expire on-time (can be
	// learned from the sector info), or in the next quantized expiration
	// epoch (i.e., the first element in the partition's expiration queue.
	// 2. If it's faulty, it will expire early within the first 14 entries
	// of the expiration queue.

	stopErr := errors.New("stop")
	out := SectorExpiration{}
	err = dls.ForEach(s.store, func(dlIdx uint64, dl *miner4.Deadline) error {
		partitions, err := dl.PartitionsArray(s.store)
		if err != nil {
			return err
		}
		quant := s.State.QuantSpecForDeadline(dlIdx)
		var part miner4.Partition
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

			q, err := miner4.LoadExpirationQueue(s.store, part.ExpirationsEpochs, quant, miner4.PartitionExpirationAmtBitwidth)
			if err != nil {
				return err
			}
			var exp miner4.ExpirationSet
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

func (s *state4) GetPrecommittedSector(num abi.SectorNumber) (*SectorPreCommitOnChainInfo, error) {
	info, ok, err := s.State.GetPrecommittedSector(s.store, num)
	if !ok || err != nil {
		return nil, err
	}

	ret := fromV4SectorPreCommitOnChainInfo(*info)

	return &ret, nil
}

func (s *state4) ForEachPrecommittedSector(cb func(SectorPreCommitOnChainInfo) error) error {
	precommitted, err := adt4.AsMap(s.store, s.State.PreCommittedSectors, builtin4.DefaultHamtBitwidth)
	if err != nil {
		return err
	}

	var info miner4.SectorPreCommitOnChainInfo
	if err := precommitted.ForEach(&info, func(_ string) error {
		return cb(fromV4SectorPreCommitOnChainInfo(info))
	}); err != nil {
		return err
	}

	return nil
}

func (s *state4) LoadSectors(snos *bitfield.BitField) ([]*SectorOnChainInfo, error) {
	sectors, err := miner4.LoadSectors(s.store, s.State.Sectors)
	if err != nil {
		return nil, err
	}

	// If no sector numbers are specified, load all.
	if snos == nil {
		infos := make([]*SectorOnChainInfo, 0, sectors.Length())
		var info4 miner4.SectorOnChainInfo
		if err := sectors.ForEach(&info4, func(_ int64) error {
			info := fromV4SectorOnChainInfo(info4)
			infos = append(infos, &info)
			return nil
		}); err != nil {
			return nil, err
		}
		return infos, nil
	}

	// Otherwise, load selected.
	infos4, err := sectors.Load(*snos)
	if err != nil {
		return nil, err
	}
	infos := make([]*SectorOnChainInfo, len(infos4))
	for i, info4 := range infos4 {
		info := fromV4SectorOnChainInfo(*info4)
		infos[i] = &info
	}
	return infos, nil
}

func (s *state4) loadAllocatedSectorNumbers() (bitfield.BitField, error) {
	var allocatedSectors bitfield.BitField
	err := s.store.Get(s.store.Context(), s.State.AllocatedSectors, &allocatedSectors)
	return allocatedSectors, err
}

func (s *state4) IsAllocated(num abi.SectorNumber) (bool, error) {
	allocatedSectors, err := s.loadAllocatedSectorNumbers()
	if err != nil {
		return false, err
	}

	return allocatedSectors.IsSet(uint64(num))
}

func (s *state4) GetProvingPeriodStart() (abi.ChainEpoch, error) {
	return s.State.ProvingPeriodStart, nil
}

func (s *state4) UnallocatedSectorNumbers(count int) ([]abi.SectorNumber, error) {
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

func (s *state4) GetAllocatedSectors() (*bitfield.BitField, error) {
	var allocatedSectors bitfield.BitField
	if err := s.store.Get(s.store.Context(), s.State.AllocatedSectors, &allocatedSectors); err != nil {
		return nil, err
	}

	return &allocatedSectors, nil
}

func (s *state4) LoadDeadline(idx uint64) (Deadline, error) {
	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return nil, err
	}
	dl, err := dls.LoadDeadline(s.store, idx)
	if err != nil {
		return nil, err
	}
	return &deadline4{*dl, s.store}, nil
}

func (s *state4) ForEachDeadline(cb func(uint64, Deadline) error) error {
	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return err
	}
	return dls.ForEach(s.store, func(i uint64, dl *miner4.Deadline) error {
		return cb(i, &deadline4{*dl, s.store})
	})
}

func (s *state4) NumDeadlines() (uint64, error) {
	return miner4.WPoStPeriodDeadlines, nil
}

func (s *state4) DeadlinesChanged(other State) (bool, error) {
	other4, ok := other.(*state4)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}

	return !s.State.Deadlines.Equals(other4.Deadlines), nil
}

func (s *state4) MinerInfoChanged(other State) (bool, error) {
	other0, ok := other.(*state4)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}
	return !s.State.Info.Equals(other0.State.Info), nil
}

func (s *state4) Info() (MinerInfo, error) {
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
	}

	return mi, nil
}

func (s *state4) DeadlineInfo(epoch abi.ChainEpoch) (*dline.Info, error) {
	return s.State.RecordedDeadlineInfo(epoch), nil
}

func (s *state4) DeadlineCronActive() (bool, error) {
	return s.State.DeadlineCronActive, nil
}

func (s *state4) sectors() (adt.Array, error) {
	return adt4.AsArray(s.store, s.Sectors, miner4.SectorsAmtBitwidth)
}

func (s *state4) decodeSectorOnChainInfo(val *cbg.Deferred) (SectorOnChainInfo, error) {
	var si miner4.SectorOnChainInfo
	err := si.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return SectorOnChainInfo{}, err
	}

	return fromV4SectorOnChainInfo(si), nil
}

func (s *state4) precommits() (adt.Map, error) {
	return adt4.AsMap(s.store, s.PreCommittedSectors, builtin4.DefaultHamtBitwidth)
}

func (s *state4) decodeSectorPreCommitOnChainInfo(val *cbg.Deferred) (SectorPreCommitOnChainInfo, error) {
	var sp miner4.SectorPreCommitOnChainInfo
	err := sp.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return SectorPreCommitOnChainInfo{}, err
	}

	return fromV4SectorPreCommitOnChainInfo(sp), nil
}

func (s *state4) EraseAllUnproven() error {

	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return err
	}

	err = dls.ForEach(s.store, func(dindx uint64, dl *miner4.Deadline) error {
		ps, err := dl.PartitionsArray(s.store)
		if err != nil {
			return err
		}

		var part miner4.Partition
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

func (d *deadline4) LoadPartition(idx uint64) (Partition, error) {
	p, err := d.Deadline.LoadPartition(d.store, idx)
	if err != nil {
		return nil, err
	}
	return &partition4{*p, d.store}, nil
}

func (d *deadline4) ForEachPartition(cb func(uint64, Partition) error) error {
	ps, err := d.Deadline.PartitionsArray(d.store)
	if err != nil {
		return err
	}
	var part miner4.Partition
	return ps.ForEach(&part, func(i int64) error {
		return cb(uint64(i), &partition4{part, d.store})
	})
}

func (d *deadline4) PartitionsChanged(other Deadline) (bool, error) {
	other4, ok := other.(*deadline4)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}

	return !d.Deadline.Partitions.Equals(other4.Deadline.Partitions), nil
}

func (d *deadline4) PartitionsPoSted() (bitfield.BitField, error) {
	return d.Deadline.PartitionsPoSted, nil
}

func (d *deadline4) DisputableProofCount() (uint64, error) {

	ops, err := d.OptimisticProofsSnapshotArray(d.store)
	if err != nil {
		return 0, err
	}

	return ops.Length(), nil

}

func (d *deadline4) DailyFee() (abi.TokenAmount, error) {
	return big.Zero(), nil
}

func (p *partition4) AllSectors() (bitfield.BitField, error) {
	return p.Partition.Sectors, nil
}

func (p *partition4) FaultySectors() (bitfield.BitField, error) {
	return p.Partition.Faults, nil
}

func (p *partition4) RecoveringSectors() (bitfield.BitField, error) {
	return p.Partition.Recoveries, nil
}

func (p *partition4) UnprovenSectors() (bitfield.BitField, error) {
	return p.Partition.Unproven, nil
}

func fromV4SectorOnChainInfo(v4 miner4.SectorOnChainInfo) SectorOnChainInfo {
	info := SectorOnChainInfo{
		SectorNumber:          v4.SectorNumber,
		SealProof:             v4.SealProof,
		SealedCID:             v4.SealedCID,
		DeprecatedDealIDs:     v4.DealIDs,
		Activation:            v4.Activation,
		Expiration:            v4.Expiration,
		DealWeight:            v4.DealWeight,
		VerifiedDealWeight:    v4.VerifiedDealWeight,
		InitialPledge:         v4.InitialPledge,
		ExpectedDayReward:     &v4.ExpectedDayReward,
		ExpectedStoragePledge: &v4.ExpectedStoragePledge,
		DailyFee:              big.Zero(),
	}
	return info
}

func fromV4SectorPreCommitOnChainInfo(v4 miner4.SectorPreCommitOnChainInfo) SectorPreCommitOnChainInfo {
	ret := SectorPreCommitOnChainInfo{
		Info: SectorPreCommitInfo{
			SealProof:     v4.Info.SealProof,
			SectorNumber:  v4.Info.SectorNumber,
			SealedCID:     v4.Info.SealedCID,
			SealRandEpoch: v4.Info.SealRandEpoch,
			DealIDs:       v4.Info.DealIDs,
			Expiration:    v4.Info.Expiration,
			UnsealedCid:   nil,
		},
		PreCommitDeposit: v4.PreCommitDeposit,
		PreCommitEpoch:   v4.PreCommitEpoch,
	}

	return ret
}

func (s *state4) GetState() interface{} {
	return &s.State
}

func (s *state4) ActorKey() string {
	return manifest.MinerKey
}

func (s *state4) ActorVersion() actorstypes.Version {
	return actorstypes.Version4
}

func (s *state4) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
