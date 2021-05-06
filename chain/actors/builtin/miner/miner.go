package miner

import (
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/dline"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"

	miner0 "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"
	miner3 "github.com/filecoin-project/specs-actors/v3/actors/builtin/miner"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"

	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"

	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"

	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
)

func init() {

	builtin.RegisterActorState(builtin0.StorageMinerActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load0(store, root)
	})

	builtin.RegisterActorState(builtin2.StorageMinerActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load2(store, root)
	})

	builtin.RegisterActorState(builtin3.StorageMinerActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load3(store, root)
	})

	builtin.RegisterActorState(builtin4.StorageMinerActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load4(store, root)
	})

	builtin.RegisterActorState(builtin5.StorageMinerActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load5(store, root)
	})

}

var Methods = builtin5.MethodsMiner

// Unchanged between v0, v2, v3, and v4 actors
var WPoStProvingPeriod = miner0.WPoStProvingPeriod
var WPoStPeriodDeadlines = miner0.WPoStPeriodDeadlines
var WPoStChallengeWindow = miner0.WPoStChallengeWindow
var WPoStChallengeLookback = miner0.WPoStChallengeLookback
var FaultDeclarationCutoff = miner0.FaultDeclarationCutoff

const MinSectorExpiration = miner0.MinSectorExpiration

// Not used / checked in v0
// TODO: Abstract over network versions
var DeclarationsMax = miner2.DeclarationsMax
var AddressedSectorsMax = miner2.AddressedSectorsMax

func Load(store adt.Store, act *types.Actor) (State, error) {
	switch act.Code {

	case builtin0.StorageMinerActorCodeID:
		return load0(store, act.Head)

	case builtin2.StorageMinerActorCodeID:
		return load2(store, act.Head)

	case builtin3.StorageMinerActorCodeID:
		return load3(store, act.Head)

	case builtin4.StorageMinerActorCodeID:
		return load4(store, act.Head)

	case builtin5.StorageMinerActorCodeID:
		return load5(store, act.Head)

	}
	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

type State interface {
	cbor.Marshaler

	// Total available balance to spend.
	AvailableBalance(abi.TokenAmount) (abi.TokenAmount, error)
	// Funds that will vest by the given epoch.
	VestedFunds(abi.ChainEpoch) (abi.TokenAmount, error)
	// Funds locked for various reasons.
	LockedFunds() (LockedFunds, error)
	FeeDebt() (abi.TokenAmount, error)

	GetSector(abi.SectorNumber) (*SectorOnChainInfo, error)
	FindSector(abi.SectorNumber) (*SectorLocation, error)
	GetSectorExpiration(abi.SectorNumber) (*SectorExpiration, error)
	GetPrecommittedSector(abi.SectorNumber) (*SectorPreCommitOnChainInfo, error)
	LoadSectors(sectorNos *bitfield.BitField) ([]*SectorOnChainInfo, error)
	NumLiveSectors() (uint64, error)
	IsAllocated(abi.SectorNumber) (bool, error)

	LoadDeadline(idx uint64) (Deadline, error)
	ForEachDeadline(cb func(idx uint64, dl Deadline) error) error
	NumDeadlines() (uint64, error)
	DeadlinesChanged(State) (bool, error)

	Info() (MinerInfo, error)
	MinerInfoChanged(State) (bool, error)

	DeadlineInfo(epoch abi.ChainEpoch) (*dline.Info, error)
	DeadlineCronActive() (bool, error)

	// Diff helpers. Used by Diff* functions internally.
	sectors() (adt.Array, error)
	decodeSectorOnChainInfo(*cbg.Deferred) (SectorOnChainInfo, error)
	precommits() (adt.Map, error)
	decodeSectorPreCommitOnChainInfo(*cbg.Deferred) (SectorPreCommitOnChainInfo, error)
}

type Deadline interface {
	LoadPartition(idx uint64) (Partition, error)
	ForEachPartition(cb func(idx uint64, part Partition) error) error
	PartitionsPoSted() (bitfield.BitField, error)

	PartitionsChanged(Deadline) (bool, error)
	DisputableProofCount() (uint64, error)
}

type Partition interface {
	AllSectors() (bitfield.BitField, error)
	FaultySectors() (bitfield.BitField, error)
	RecoveringSectors() (bitfield.BitField, error)
	LiveSectors() (bitfield.BitField, error)
	ActiveSectors() (bitfield.BitField, error)
}

type SectorOnChainInfo struct {
	SectorNumber          abi.SectorNumber
	SealProof             abi.RegisteredSealProof
	SealedCID             cid.Cid
	DealIDs               []abi.DealID
	Activation            abi.ChainEpoch
	Expiration            abi.ChainEpoch
	DealWeight            abi.DealWeight
	VerifiedDealWeight    abi.DealWeight
	InitialPledge         abi.TokenAmount
	ExpectedDayReward     abi.TokenAmount
	ExpectedStoragePledge abi.TokenAmount
}

type SectorPreCommitInfo = miner0.SectorPreCommitInfo

type SectorPreCommitOnChainInfo struct {
	Info               SectorPreCommitInfo
	PreCommitDeposit   abi.TokenAmount
	PreCommitEpoch     abi.ChainEpoch
	DealWeight         abi.DealWeight
	VerifiedDealWeight abi.DealWeight
}

type PoStPartition = miner0.PoStPartition
type RecoveryDeclaration = miner0.RecoveryDeclaration
type FaultDeclaration = miner0.FaultDeclaration

// Params
type DeclareFaultsParams = miner0.DeclareFaultsParams
type DeclareFaultsRecoveredParams = miner0.DeclareFaultsRecoveredParams
type SubmitWindowedPoStParams = miner0.SubmitWindowedPoStParams
type ProveCommitSectorParams = miner0.ProveCommitSectorParams
type DisputeWindowedPoStParams = miner3.DisputeWindowedPoStParams

func PreferredSealProofTypeFromWindowPoStType(nver network.Version, proof abi.RegisteredPoStProof) (abi.RegisteredSealProof, error) {
	// We added support for the new proofs in network version 7, and removed support for the old
	// ones in network version 8.
	if nver < network.Version7 {
		switch proof {
		case abi.RegisteredPoStProof_StackedDrgWindow2KiBV1:
			return abi.RegisteredSealProof_StackedDrg2KiBV1, nil
		case abi.RegisteredPoStProof_StackedDrgWindow8MiBV1:
			return abi.RegisteredSealProof_StackedDrg8MiBV1, nil
		case abi.RegisteredPoStProof_StackedDrgWindow512MiBV1:
			return abi.RegisteredSealProof_StackedDrg512MiBV1, nil
		case abi.RegisteredPoStProof_StackedDrgWindow32GiBV1:
			return abi.RegisteredSealProof_StackedDrg32GiBV1, nil
		case abi.RegisteredPoStProof_StackedDrgWindow64GiBV1:
			return abi.RegisteredSealProof_StackedDrg64GiBV1, nil
		default:
			return -1, xerrors.Errorf("unrecognized window post type: %d", proof)
		}
	}

	switch proof {
	case abi.RegisteredPoStProof_StackedDrgWindow2KiBV1:
		return abi.RegisteredSealProof_StackedDrg2KiBV1_1, nil
	case abi.RegisteredPoStProof_StackedDrgWindow8MiBV1:
		return abi.RegisteredSealProof_StackedDrg8MiBV1_1, nil
	case abi.RegisteredPoStProof_StackedDrgWindow512MiBV1:
		return abi.RegisteredSealProof_StackedDrg512MiBV1_1, nil
	case abi.RegisteredPoStProof_StackedDrgWindow32GiBV1:
		return abi.RegisteredSealProof_StackedDrg32GiBV1_1, nil
	case abi.RegisteredPoStProof_StackedDrgWindow64GiBV1:
		return abi.RegisteredSealProof_StackedDrg64GiBV1_1, nil
	default:
		return -1, xerrors.Errorf("unrecognized window post type: %d", proof)
	}
}

func WinningPoStProofTypeFromWindowPoStProofType(nver network.Version, proof abi.RegisteredPoStProof) (abi.RegisteredPoStProof, error) {
	switch proof {
	case abi.RegisteredPoStProof_StackedDrgWindow2KiBV1:
		return abi.RegisteredPoStProof_StackedDrgWinning2KiBV1, nil
	case abi.RegisteredPoStProof_StackedDrgWindow8MiBV1:
		return abi.RegisteredPoStProof_StackedDrgWinning8MiBV1, nil
	case abi.RegisteredPoStProof_StackedDrgWindow512MiBV1:
		return abi.RegisteredPoStProof_StackedDrgWinning512MiBV1, nil
	case abi.RegisteredPoStProof_StackedDrgWindow32GiBV1:
		return abi.RegisteredPoStProof_StackedDrgWinning32GiBV1, nil
	case abi.RegisteredPoStProof_StackedDrgWindow64GiBV1:
		return abi.RegisteredPoStProof_StackedDrgWinning64GiBV1, nil
	default:
		return -1, xerrors.Errorf("unknown proof type %d", proof)
	}
}

type MinerInfo struct {
	Owner                      address.Address   // Must be an ID-address.
	Worker                     address.Address   // Must be an ID-address.
	NewWorker                  address.Address   // Must be an ID-address.
	ControlAddresses           []address.Address // Must be an ID-addresses.
	WorkerChangeEpoch          abi.ChainEpoch
	PeerId                     *peer.ID
	Multiaddrs                 []abi.Multiaddrs
	WindowPoStProofType        abi.RegisteredPoStProof
	SectorSize                 abi.SectorSize
	WindowPoStPartitionSectors uint64
	ConsensusFaultElapsed      abi.ChainEpoch
}

func (mi MinerInfo) IsController(addr address.Address) bool {
	if addr == mi.Owner || addr == mi.Worker {
		return true
	}

	for _, ca := range mi.ControlAddresses {
		if addr == ca {
			return true
		}
	}

	return false
}

type SectorExpiration struct {
	OnTime abi.ChainEpoch

	// non-zero if sector is faulty, epoch at which it will be permanently
	// removed if it doesn't recover
	Early abi.ChainEpoch
}

type SectorLocation struct {
	Deadline  uint64
	Partition uint64
}

type SectorChanges struct {
	Added    []SectorOnChainInfo
	Extended []SectorExtensions
	Removed  []SectorOnChainInfo
}

type SectorExtensions struct {
	From SectorOnChainInfo
	To   SectorOnChainInfo
}

type PreCommitChanges struct {
	Added   []SectorPreCommitOnChainInfo
	Removed []SectorPreCommitOnChainInfo
}

type LockedFunds struct {
	VestingFunds             abi.TokenAmount
	InitialPledgeRequirement abi.TokenAmount
	PreCommitDeposits        abi.TokenAmount
}

func (lf LockedFunds) TotalLockedFunds() abi.TokenAmount {
	return big.Add(lf.VestingFunds, big.Add(lf.InitialPledgeRequirement, lf.PreCommitDeposits))
}
