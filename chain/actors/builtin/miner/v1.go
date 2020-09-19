package miner

import (
	"bytes"
	"errors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	adt1 "github.com/filecoin-project/specs-actors/v2/actors/util/adt"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	miner1 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"
)

var _ State = (*state1)(nil)

type state1 struct {
	miner1.State
	store adt.Store
}

type deadline1 struct {
	miner1.Deadline
	store adt.Store
}

type partition1 struct {
	miner1.Partition
	store adt.Store
}

func (s *state1) AvailableBalance(bal abi.TokenAmount) (abi.TokenAmount, error) {
	return s.GetAvailableBalance(bal), nil
}

func (s *state1) VestedFunds(epoch abi.ChainEpoch) (abi.TokenAmount, error) {
	return s.CheckVestedFunds(s.store, epoch)
}

func (s *state1) LockedFunds() (LockedFunds, error) {
	return LockedFunds{
		VestingFunds:             s.State.LockedFunds,
		InitialPledgeRequirement: s.State.InitialPledge,
		PreCommitDeposits:        s.State.PreCommitDeposits,
	}, nil
}

func (s *state1) InitialPledge() (abi.TokenAmount, error) {
	return s.State.InitialPledge, nil
}

func (s *state1) PreCommitDeposits() (abi.TokenAmount, error) {
	return s.State.PreCommitDeposits, nil
}

func (s *state1) GetSector(num abi.SectorNumber) (*SectorOnChainInfo, error) {
	info, ok, err := s.State.GetSector(s.store, num)
	if !ok || err != nil {
		return nil, err
	}

	ret := fromV1SectorOnChainInfo(*info)
	return &ret, nil
}

func (s *state1) FindSector(num abi.SectorNumber) (*SectorLocation, error) {
	dlIdx, partIdx, err := s.State.FindSector(s.store, num)
	if err != nil {
		return nil, err
	}
	return &SectorLocation{
		Deadline:  dlIdx,
		Partition: partIdx,
	}, nil
}

// GetSectorExpiration returns the effective expiration of the given sector.
//
// If the sector isn't found or has already been terminated, this method returns
// nil and no error. If the sector does not expire early, the Early expiration
// field is 0.
func (s *state1) GetSectorExpiration(num abi.SectorNumber) (*SectorExpiration, error) {
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
	err = dls.ForEach(s.store, func(dlIdx uint64, dl *miner1.Deadline) error {
		partitions, err := dl.PartitionsArray(s.store)
		if err != nil {
			return err
		}
		quant := s.State.QuantSpecForDeadline(dlIdx)
		var part miner1.Partition
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

			q, err := miner1.LoadExpirationQueue(s.store, part.ExpirationsEpochs, quant)
			if err != nil {
				return err
			}
			var exp miner1.ExpirationSet
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
		return nil, nil
	}
	return &out, nil
}

func (s *state1) GetPrecommittedSector(num abi.SectorNumber) (*SectorPreCommitOnChainInfo, error) {
	info, ok, err := s.State.GetPrecommittedSector(s.store, num)
	if !ok || err != nil {
		return nil, err
	}

	ret := fromV1SectorPreCommitOnChainInfo(*info)

	return &ret, nil
}

func (s *state1) LoadSectorsFromSet(filter *bitfield.BitField, filterOut bool) (adt.Array, error) {
	a, err := adt1.AsArray(s.store, s.State.Sectors)
	if err != nil {
		return nil, err
	}

	ret := adt1.MakeEmptyArray(s.store)
	var v cbg.Deferred
	if err := a.ForEach(&v, func(i int64) error {
		include := true
		if filter != nil {
			set, err := filter.IsSet(uint64(i))
			if err != nil {
				return xerrors.Errorf("filter check error: %w", err)
			}
			if set == filterOut {
				include = false
			}
		}

		if include {
			var oci miner1.SectorOnChainInfo
			if err := oci.UnmarshalCBOR(bytes.NewReader(v.Raw)); err != nil {
				return err
			}

			noci := SectorOnChainInfo{
				SectorNumber:          oci.SectorNumber,
				SealProof:             oci.SealProof,
				SealedCID:             oci.SealedCID,
				DealIDs:               oci.DealIDs,
				Activation:            oci.Activation,
				Expiration:            oci.Expiration,
				DealWeight:            oci.DealWeight,
				VerifiedDealWeight:    oci.VerifiedDealWeight,
				InitialPledge:         oci.InitialPledge,
				ExpectedDayReward:     oci.ExpectedDayReward,
				ExpectedStoragePledge: oci.ExpectedStoragePledge,
			}

			if err := ret.Set(uint64(i), &noci); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return ret, nil
}

func (s *state1) LoadPreCommittedSectors() (adt.Map, error) {
	return adt1.AsMap(s.store, s.State.PreCommittedSectors)
}

func (s *state1) IsAllocated(num abi.SectorNumber) (bool, error) {
	var allocatedSectors bitfield.BitField
	if err := s.store.Get(s.store.Context(), s.State.AllocatedSectors, &allocatedSectors); err != nil {
		return false, err
	}

	return allocatedSectors.IsSet(uint64(num))
}

func (s *state1) LoadDeadline(idx uint64) (Deadline, error) {
	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return nil, err
	}
	dl, err := dls.LoadDeadline(s.store, idx)
	if err != nil {
		return nil, err
	}
	return &deadline1{*dl, s.store}, nil
}

func (s *state1) ForEachDeadline(cb func(uint64, Deadline) error) error {
	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return err
	}
	return dls.ForEach(s.store, func(i uint64, dl *miner1.Deadline) error {
		return cb(i, &deadline1{*dl, s.store})
	})
}

func (s *state1) NumDeadlines() (uint64, error) {
	return miner1.WPoStPeriodDeadlines, nil
}

func (s *state1) DeadlinesChanged(other State) bool {
	other1, ok := other.(*state1)
	if !ok {
		// treat an upgrade as a change, always
		return true
	}

	return s.State.Deadlines.Equals(other1.Deadlines)
}

func (s *state1) Info() (MinerInfo, error) {
	info, err := s.State.GetInfo(s.store)
	if err != nil {
		return MinerInfo{}, err
	}

	var pid *peer.ID
	if peerID, err := peer.IDFromBytes(info.PeerId); err == nil {
		pid = &peerID
	}

	mi := MinerInfo{
		Owner:            info.Owner,
		Worker:           info.Worker,
		ControlAddresses: info.ControlAddresses,

		NewWorker:         address.Undef,
		WorkerChangeEpoch: -1,

		PeerId:                     pid,
		Multiaddrs:                 info.Multiaddrs,
		SealProofType:              info.SealProofType,
		SectorSize:                 info.SectorSize,
		WindowPoStPartitionSectors: info.WindowPoStPartitionSectors,
	}

	if info.PendingWorkerKey != nil {
		mi.NewWorker = info.PendingWorkerKey.NewWorker
		mi.WorkerChangeEpoch = info.PendingWorkerKey.EffectiveAt
	}

	return mi, nil
}

func (s *state1) DeadlineInfo(epoch abi.ChainEpoch) *dline.Info {
	return s.State.DeadlineInfo(epoch)
}

func (s *state1) sectors() (adt.Array, error) {
	return adt1.AsArray(s.store, s.Sectors)
}

func (s *state1) decodeSectorOnChainInfo(val *cbg.Deferred) (SectorOnChainInfo, error) {
	var si miner1.SectorOnChainInfo
	err := si.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return SectorOnChainInfo{}, err
	}

	ret := fromV1SectorOnChainInfo(si)
	return ret, nil
}

func (s *state1) precommits() (adt.Map, error) {
	return adt1.AsMap(s.store, s.PreCommittedSectors)
}

func (s *state1) decodeSectorPreCommitOnChainInfo(val *cbg.Deferred) (SectorPreCommitOnChainInfo, error) {
	var sp miner1.SectorPreCommitOnChainInfo
	err := sp.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return SectorPreCommitOnChainInfo{}, err
	}

	ret := fromV1SectorPreCommitOnChainInfo(sp)
	return ret, nil
}

func (d *deadline1) LoadPartition(idx uint64) (Partition, error) {
	p, err := d.Deadline.LoadPartition(d.store, idx)
	if err != nil {
		return nil, err
	}
	return &partition1{*p, d.store}, nil
}

func (d *deadline1) ForEachPartition(cb func(uint64, Partition) error) error {
	ps, err := d.Deadline.PartitionsArray(d.store)
	if err != nil {
		return err
	}
	var part miner1.Partition
	return ps.ForEach(&part, func(i int64) error {
		return cb(uint64(i), &partition1{part, d.store})
	})
}

func (d *deadline1) PartitionsChanged(other Deadline) bool {
	other1, ok := other.(*deadline1)
	if !ok {
		// treat an upgrade as a change, always
		return true
	}

	return d.Deadline.Partitions.Equals(other1.Deadline.Partitions)
}

func (d *deadline1) PostSubmissions() (bitfield.BitField, error) {
	return d.Deadline.PostSubmissions, nil
}

func (p *partition1) AllSectors() (bitfield.BitField, error) {
	return p.Partition.Sectors, nil
}

func (p *partition1) FaultySectors() (bitfield.BitField, error) {
	return p.Partition.Faults, nil
}

func (p *partition1) RecoveringSectors() (bitfield.BitField, error) {
	return p.Partition.Recoveries, nil
}

func fromV1SectorOnChainInfo(v1 miner1.SectorOnChainInfo) SectorOnChainInfo {
	return SectorOnChainInfo{
		SectorNumber:          v1.SectorNumber,
		SealProof:             v1.SealProof,
		SealedCID:             v1.SealedCID,
		DealIDs:               v1.DealIDs,
		Activation:            v1.Activation,
		Expiration:            v1.Expiration,
		DealWeight:            v1.DealWeight,
		VerifiedDealWeight:    v1.VerifiedDealWeight,
		InitialPledge:         v1.InitialPledge,
		ExpectedDayReward:     v1.ExpectedDayReward,
		ExpectedStoragePledge: v1.ExpectedStoragePledge,
	}
}

func fromV1SectorPreCommitOnChainInfo(v1 miner1.SectorPreCommitOnChainInfo) SectorPreCommitOnChainInfo {
	return SectorPreCommitOnChainInfo{
		Info:               (SectorPreCommitInfo)(v1.Info),
		PreCommitDeposit:   v1.PreCommitDeposit,
		PreCommitEpoch:     v1.PreCommitEpoch,
		DealWeight:         v1.DealWeight,
		VerifiedDealWeight: v1.VerifiedDealWeight,
	}
}
