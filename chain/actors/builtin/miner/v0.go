package miner

import (
	"bytes"
	"errors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	v0adt "github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	v0miner "github.com/filecoin-project/specs-actors/actors/builtin/miner"
)

type v0State struct {
	v0miner.State
	store adt.Store
}

type v0Deadline struct {
	v0miner.Deadline
	store adt.Store
}

type v0Partition struct {
	v0miner.Partition
	store adt.Store
}

func (s *v0State) AvailableBalance(bal abi.TokenAmount) (abi.TokenAmount, error) {
	return s.GetAvailableBalance(bal), nil
}

func (s *v0State) VestedFunds(epoch abi.ChainEpoch) (abi.TokenAmount, error) {
	return s.CheckVestedFunds(s.store, epoch)
}

func (s *v0State) GetSector(num abi.SectorNumber) (*SectorOnChainInfo, error) {
	info, ok, err := s.State.GetSector(s.store, num)
	if !ok || err != nil {
		return nil, err
	}

	return info, nil
}

func (s *v0State) FindSector(num abi.SectorNumber) (*SectorLocation, error) {
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
func (s *v0State) GetSectorExpiration(num abi.SectorNumber) (*SectorExpiration, error) {
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
	err = dls.ForEach(s.store, func(dlIdx uint64, dl *v0miner.Deadline) error {
		partitions, err := dl.PartitionsArray(s.store)
		if err != nil {
			return err
		}
		quant := s.State.QuantSpecForDeadline(dlIdx)
		var part v0miner.Partition
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

			q, err := v0miner.LoadExpirationQueue(s.store, part.ExpirationsEpochs, quant)
			if err != nil {
				return err
			}
			var exp v0miner.ExpirationSet
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

func (s *v0State) GetPrecommittedSector(num abi.SectorNumber) (*SectorPreCommitOnChainInfo, error) {
	info, ok, err := s.State.GetPrecommittedSector(s.store, num)
	if !ok || err != nil {
		return nil, err
	}

	return info, nil
}

func (s *v0State) LoadSectorsFromSet(filter *bitfield.BitField, filterOut bool) (adt.Array, error) {
	a, err := v0adt.AsArray(s.store, s.State.Sectors)
	if err != nil {
		return nil, err
	}

	var v cbg.Deferred
	if err := a.ForEach(&v, func(i int64) error {
		if filter != nil {
			set, err := filter.IsSet(uint64(i))
			if err != nil {
				return xerrors.Errorf("filter check error: %w", err)
			}
			if set == filterOut {
				err = a.Delete(uint64(i))
				if err != nil {
					return xerrors.Errorf("filtering error: %w", err)
				}
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return a, nil
}

func (s *v0State) LoadPreCommittedSectors() (adt.Map, error) {
	return v0adt.AsMap(s.store, s.State.PreCommittedSectors)
}

func (s *v0State) IsAllocated(num abi.SectorNumber) (bool, error) {
	var allocatedSectors bitfield.BitField
	if err := s.store.Get(s.store.Context(), s.State.AllocatedSectors, &allocatedSectors); err != nil {
		return false, err
	}

	return allocatedSectors.IsSet(uint64(num))
}

func (s *v0State) LoadDeadline(idx uint64) (Deadline, error) {
	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return nil, err
	}
	dl, err := dls.LoadDeadline(s.store, idx)
	if err != nil {
		return nil, err
	}
	return &v0Deadline{*dl, s.store}, nil
}

func (s *v0State) ForEachDeadline(cb func(uint64, Deadline) error) error {
	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return err
	}
	return dls.ForEach(s.store, func(i uint64, dl *v0miner.Deadline) error {
		return cb(i, &v0Deadline{*dl, s.store})
	})
}

func (s *v0State) NumDeadlines() (uint64, error) {
	return v0miner.WPoStPeriodDeadlines, nil
}

func (s *v0State) DeadlinesChanged(other State) bool {
	v0other, ok := other.(*v0State)
	if !ok {
		// treat an upgrade as a change, always
		return true
	}

	return s.State.Deadlines.Equals(v0other.Deadlines)
}

func (s *v0State) Info() (MinerInfo, error) {
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

func (s *v0State) DeadlineInfo(epoch abi.ChainEpoch) *dline.Info {
	return s.State.DeadlineInfo(epoch)
}

func (s *v0State) sectors() (adt.Array, error) {
	return v0adt.AsArray(s.store, s.Sectors)
}

func (s *v0State) decodeSectorOnChainInfo(val *cbg.Deferred) (SectorOnChainInfo, error) {
	var si v0miner.SectorOnChainInfo
	err := si.UnmarshalCBOR(bytes.NewReader(val.Raw))
	return si, err
}

func (s *v0State) precommits() (adt.Map, error) {
	return v0adt.AsMap(s.store, s.PreCommittedSectors)
}

func (s *v0State) decodeSectorPreCommitOnChainInfo(val *cbg.Deferred) (SectorPreCommitOnChainInfo, error) {
	var sp v0miner.SectorPreCommitOnChainInfo
	err := sp.UnmarshalCBOR(bytes.NewReader(val.Raw))
	return sp, err
}

func (d *v0Deadline) LoadPartition(idx uint64) (Partition, error) {
	p, err := d.Deadline.LoadPartition(d.store, idx)
	if err != nil {
		return nil, err
	}
	return &v0Partition{*p, d.store}, nil
}

func (d *v0Deadline) ForEachPartition(cb func(uint64, Partition) error) error {
	ps, err := d.Deadline.PartitionsArray(d.store)
	if err != nil {
		return err
	}
	var part v0miner.Partition
	return ps.ForEach(&part, func(i int64) error {
		return cb(uint64(i), &v0Partition{part, d.store})
	})
}

func (d *v0Deadline) PartitionsChanged(other Deadline) bool {
	v0other, ok := other.(*v0Deadline)
	if !ok {
		// treat an upgrade as a change, always
		return true
	}

	return d.Deadline.Partitions.Equals(v0other.Deadline.Partitions)
}

func (d *v0Deadline) PostSubmissions() (bitfield.BitField, error) {
	return d.Deadline.PostSubmissions, nil
}

func (p *v0Partition) AllSectors() (bitfield.BitField, error) {
	return p.Partition.Sectors, nil
}

func (p *v0Partition) FaultySectors() (bitfield.BitField, error) {
	return p.Partition.Faults, nil
}

func (p *v0Partition) RecoveringSectors() (bitfield.BitField, error) {
	return p.Partition.Recoveries, nil
}
