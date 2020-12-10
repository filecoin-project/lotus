package miner

import (
	"bytes"
	"errors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors/adt"

	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"
	adt2 "github.com/filecoin-project/specs-actors/v2/actors/util/adt"
)

var _ State = (*state2)(nil)

func load2(store adt.Store, root cid.Cid) (State, error) {
	out := state2{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

type state2 struct {
	miner2.State
	store adt.Store
}

type deadline2 struct {
	miner2.Deadline
	store adt.Store
}

type partition2 struct {
	miner2.Partition
	store adt.Store
}

func (s *state2) AvailableBalance(bal abi.TokenAmount) (available abi.TokenAmount, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = xerrors.Errorf("failed to get available balance: %w", r)
			available = abi.NewTokenAmount(0)
		}
	}()
	// this panics if the miner doesnt have enough funds to cover their locked pledge
	available, err = s.GetAvailableBalance(bal)
	return available, err
}

func (s *state2) VestedFunds(epoch abi.ChainEpoch) (abi.TokenAmount, error) {
	return s.CheckVestedFunds(s.store, epoch)
}

func (s *state2) LockedFunds() (LockedFunds, error) {
	return LockedFunds{
		VestingFunds:             s.State.LockedFunds,
		InitialPledgeRequirement: s.State.InitialPledge,
		PreCommitDeposits:        s.State.PreCommitDeposits,
	}, nil
}

func (s *state2) FeeDebt() (abi.TokenAmount, error) {
	return s.State.FeeDebt, nil
}

func (s *state2) InitialPledge() (abi.TokenAmount, error) {
	return s.State.InitialPledge, nil
}

func (s *state2) PreCommitDeposits() (abi.TokenAmount, error) {
	return s.State.PreCommitDeposits, nil
}

func (s *state2) GetSector(num abi.SectorNumber) (*SectorOnChainInfo, error) {
	info, ok, err := s.State.GetSector(s.store, num)
	if !ok || err != nil {
		return nil, err
	}

	ret := fromV2SectorOnChainInfo(*info)
	return &ret, nil
}

func (s *state2) FindSector(num abi.SectorNumber) (*SectorLocation, error) {
	dlIdx, partIdx, err := s.State.FindSector(s.store, num)
	if err != nil {
		return nil, err
	}
	return &SectorLocation{
		Deadline:  dlIdx,
		Partition: partIdx,
	}, nil
}

func (s *state2) NumLiveSectors() (uint64, error) {
	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return 0, err
	}
	var total uint64
	if err := dls.ForEach(s.store, func(dlIdx uint64, dl *miner2.Deadline) error {
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
func (s *state2) GetSectorExpiration(num abi.SectorNumber) (*SectorExpiration, error) {
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
	err = dls.ForEach(s.store, func(dlIdx uint64, dl *miner2.Deadline) error {
		partitions, err := dl.PartitionsArray(s.store)
		if err != nil {
			return err
		}
		quant := s.State.QuantSpecForDeadline(dlIdx)
		var part miner2.Partition
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

			q, err := miner2.LoadExpirationQueue(s.store, part.ExpirationsEpochs, quant)
			if err != nil {
				return err
			}
			var exp miner2.ExpirationSet
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

func (s *state2) GetPrecommittedSector(num abi.SectorNumber) (*SectorPreCommitOnChainInfo, error) {
	info, ok, err := s.State.GetPrecommittedSector(s.store, num)
	if !ok || err != nil {
		return nil, err
	}

	ret := fromV2SectorPreCommitOnChainInfo(*info)

	return &ret, nil
}

func (s *state2) LoadSectors(snos *bitfield.BitField) ([]*SectorOnChainInfo, error) {
	sectors, err := miner2.LoadSectors(s.store, s.State.Sectors)
	if err != nil {
		return nil, err
	}

	// If no sector numbers are specified, load all.
	if snos == nil {
		infos := make([]*SectorOnChainInfo, 0, sectors.Length())
		var info2 miner2.SectorOnChainInfo
		if err := sectors.ForEach(&info2, func(_ int64) error {
			info := fromV2SectorOnChainInfo(info2)
			infos = append(infos, &info)
			return nil
		}); err != nil {
			return nil, err
		}
		return infos, nil
	}

	// Otherwise, load selected.
	infos2, err := sectors.Load(*snos)
	if err != nil {
		return nil, err
	}
	infos := make([]*SectorOnChainInfo, len(infos2))
	for i, info2 := range infos2 {
		info := fromV2SectorOnChainInfo(*info2)
		infos[i] = &info
	}
	return infos, nil
}

func (s *state2) IsAllocated(num abi.SectorNumber) (bool, error) {
	var allocatedSectors bitfield.BitField
	if err := s.store.Get(s.store.Context(), s.State.AllocatedSectors, &allocatedSectors); err != nil {
		return false, err
	}

	return allocatedSectors.IsSet(uint64(num))
}

func (s *state2) LoadDeadline(idx uint64) (Deadline, error) {
	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return nil, err
	}
	dl, err := dls.LoadDeadline(s.store, idx)
	if err != nil {
		return nil, err
	}
	return &deadline2{*dl, s.store}, nil
}

func (s *state2) ForEachDeadline(cb func(uint64, Deadline) error) error {
	dls, err := s.State.LoadDeadlines(s.store)
	if err != nil {
		return err
	}
	return dls.ForEach(s.store, func(i uint64, dl *miner2.Deadline) error {
		return cb(i, &deadline2{*dl, s.store})
	})
}

func (s *state2) NumDeadlines() (uint64, error) {
	return miner2.WPoStPeriodDeadlines, nil
}

func (s *state2) DeadlinesChanged(other State) (bool, error) {
	other2, ok := other.(*state2)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}

	return !s.State.Deadlines.Equals(other2.Deadlines), nil
}

func (s *state2) MinerInfoChanged(other State) (bool, error) {
	other0, ok := other.(*state2)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}
	return !s.State.Info.Equals(other0.State.Info), nil
}

func (s *state2) Info() (MinerInfo, error) {
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
		ConsensusFaultElapsed:      info.ConsensusFaultElapsed,
	}

	if info.PendingWorkerKey != nil {
		mi.NewWorker = info.PendingWorkerKey.NewWorker
		mi.WorkerChangeEpoch = info.PendingWorkerKey.EffectiveAt
	}

	return mi, nil
}

func (s *state2) DeadlineInfo(epoch abi.ChainEpoch) (*dline.Info, error) {
	return s.State.DeadlineInfo(epoch), nil
}

func (s *state2) sectors() (adt.Array, error) {
	return adt2.AsArray(s.store, s.Sectors)
}

func (s *state2) decodeSectorOnChainInfo(val *cbg.Deferred) (SectorOnChainInfo, error) {
	var si miner2.SectorOnChainInfo
	err := si.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return SectorOnChainInfo{}, err
	}

	return fromV2SectorOnChainInfo(si), nil
}

func (s *state2) precommits() (adt.Map, error) {
	return adt2.AsMap(s.store, s.PreCommittedSectors)
}

func (s *state2) decodeSectorPreCommitOnChainInfo(val *cbg.Deferred) (SectorPreCommitOnChainInfo, error) {
	var sp miner2.SectorPreCommitOnChainInfo
	err := sp.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return SectorPreCommitOnChainInfo{}, err
	}

	return fromV2SectorPreCommitOnChainInfo(sp), nil
}

func (d *deadline2) LoadPartition(idx uint64) (Partition, error) {
	p, err := d.Deadline.LoadPartition(d.store, idx)
	if err != nil {
		return nil, err
	}
	return &partition2{*p, d.store}, nil
}

func (d *deadline2) ForEachPartition(cb func(uint64, Partition) error) error {
	ps, err := d.Deadline.PartitionsArray(d.store)
	if err != nil {
		return err
	}
	var part miner2.Partition
	return ps.ForEach(&part, func(i int64) error {
		return cb(uint64(i), &partition2{part, d.store})
	})
}

func (d *deadline2) PartitionsChanged(other Deadline) (bool, error) {
	other2, ok := other.(*deadline2)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}

	return !d.Deadline.Partitions.Equals(other2.Deadline.Partitions), nil
}

func (d *deadline2) PostSubmissions() (bitfield.BitField, error) {
	return d.Deadline.PostSubmissions, nil
}

func (p *partition2) AllSectors() (bitfield.BitField, error) {
	return p.Partition.Sectors, nil
}

func (p *partition2) FaultySectors() (bitfield.BitField, error) {
	return p.Partition.Faults, nil
}

func (p *partition2) RecoveringSectors() (bitfield.BitField, error) {
	return p.Partition.Recoveries, nil
}

func fromV2SectorOnChainInfo(v2 miner2.SectorOnChainInfo) SectorOnChainInfo {
	return SectorOnChainInfo{
		SectorNumber:          v2.SectorNumber,
		SealProof:             v2.SealProof,
		SealedCID:             v2.SealedCID,
		DealIDs:               v2.DealIDs,
		Activation:            v2.Activation,
		Expiration:            v2.Expiration,
		DealWeight:            v2.DealWeight,
		VerifiedDealWeight:    v2.VerifiedDealWeight,
		InitialPledge:         v2.InitialPledge,
		ExpectedDayReward:     v2.ExpectedDayReward,
		ExpectedStoragePledge: v2.ExpectedStoragePledge,
	}
}

func fromV2SectorPreCommitOnChainInfo(v2 miner2.SectorPreCommitOnChainInfo) SectorPreCommitOnChainInfo {
	return SectorPreCommitOnChainInfo{
		Info:               (SectorPreCommitInfo)(v2.Info),
		PreCommitDeposit:   v2.PreCommitDeposit,
		PreCommitEpoch:     v2.PreCommitEpoch,
		DealWeight:         v2.DealWeight,
		VerifiedDealWeight: v2.VerifiedDealWeight,
	}
}
