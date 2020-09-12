package miner

import (
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
)

type v0State struct {
	miner.State
	store adt.Store
}

type v0Deadline struct {
	miner.Deadline
	store adt.Store
}

type v0Partition struct {
	miner.Partition
	store adt.Store
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
	return dls.ForEach(s.store, func(i uint64, dl *miner.Deadline) error {
		return cb(i, &v0Deadline{*dl, s.store})
	})
}

func (s *v0State) NumDeadlines() (uint64, error) {
	return miner.WPoStPeriodDeadlines, nil
}

func (s *v0State) Info() (MinerInfo, error) {
	info, err := s.State.GetInfo(s.store)

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

	return mi
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
	var part miner.Partition
	return ps.ForEach(&part, func(i int64) error {
		return cb(uint64(i), &v0Partition{part, d.store})
	})
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
