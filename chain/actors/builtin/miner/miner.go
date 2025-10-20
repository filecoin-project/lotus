package miner

import (
	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	gstbuiltin "github.com/filecoin-project/go-state-types/builtin"
	minertypes13 "github.com/filecoin-project/go-state-types/builtin/v13/miner"
	smoothing13 "github.com/filecoin-project/go-state-types/builtin/v13/util/smoothing"
	minertypes14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	smoothing14 "github.com/filecoin-project/go-state-types/builtin/v14/util/smoothing"
	minertypes15 "github.com/filecoin-project/go-state-types/builtin/v15/miner"
	smoothing15 "github.com/filecoin-project/go-state-types/builtin/v15/util/smoothing"
	minertypes16 "github.com/filecoin-project/go-state-types/builtin/v16/miner"
	smoothing16 "github.com/filecoin-project/go-state-types/builtin/v16/util/smoothing"
	minertypes17 "github.com/filecoin-project/go-state-types/builtin/v17/miner"
	smoothing17 "github.com/filecoin-project/go-state-types/builtin/v17/util/smoothing"
	minertypes18 "github.com/filecoin-project/go-state-types/builtin/v18/miner"
	smoothing18 "github.com/filecoin-project/go-state-types/builtin/v18/util/smoothing"
	minertypes "github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/manifest"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/go-state-types/proof"
	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"
	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"
	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"
	builtin7 "github.com/filecoin-project/specs-actors/v7/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
)

var Methods = gstbuiltin.MethodsMiner

func Load(store adt.Store, act *types.Actor) (State, error) {
	if name, av, ok := actors.GetActorMetaByCode(act.Code); ok {
		if name != manifest.MinerKey {
			return nil, xerrors.Errorf("actor code is not miner: %s", name)
		}

		switch av {

		case actorstypes.Version8:
			return load8(store, act.Head)

		case actorstypes.Version9:
			return load9(store, act.Head)

		case actorstypes.Version10:
			return load10(store, act.Head)

		case actorstypes.Version11:
			return load11(store, act.Head)

		case actorstypes.Version12:
			return load12(store, act.Head)

		case actorstypes.Version13:
			return load13(store, act.Head)

		case actorstypes.Version14:
			return load14(store, act.Head)

		case actorstypes.Version15:
			return load15(store, act.Head)

		case actorstypes.Version16:
			return load16(store, act.Head)

		case actorstypes.Version17:
			return load17(store, act.Head)

		case actorstypes.Version18:
			return load18(store, act.Head)

		}
	}

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

	case builtin6.StorageMinerActorCodeID:
		return load6(store, act.Head)

	case builtin7.StorageMinerActorCodeID:
		return load7(store, act.Head)

	}

	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

func MakeState(store adt.Store, av actors.Version) (State, error) {
	switch av {

	case actors.Version0:
		return make0(store)

	case actors.Version2:
		return make2(store)

	case actors.Version3:
		return make3(store)

	case actors.Version4:
		return make4(store)

	case actors.Version5:
		return make5(store)

	case actors.Version6:
		return make6(store)

	case actors.Version7:
		return make7(store)

	case actors.Version8:
		return make8(store)

	case actors.Version9:
		return make9(store)

	case actors.Version10:
		return make10(store)

	case actors.Version11:
		return make11(store)

	case actors.Version12:
		return make12(store)

	case actors.Version13:
		return make13(store)

	case actors.Version14:
		return make14(store)

	case actors.Version15:
		return make15(store)

	case actors.Version16:
		return make16(store)

	case actors.Version17:
		return make17(store)

	case actors.Version18:
		return make18(store)

	}
	return nil, xerrors.Errorf("unknown actor version %d", av)
}

type State interface {
	cbor.Marshaler

	Code() cid.Cid
	ActorKey() string
	ActorVersion() actorstypes.Version

	// Total available balance to spend.
	AvailableBalance(abi.TokenAmount) (abi.TokenAmount, error)
	// Funds that will vest by the given epoch.
	VestedFunds(abi.ChainEpoch) (abi.TokenAmount, error)
	// Funds locked for various reasons.
	LockedFunds() (LockedFunds, error)
	FeeDebt() (abi.TokenAmount, error)
	InitialPledge() (abi.TokenAmount, error)

	// Returns nil, nil if sector is not found
	GetSector(abi.SectorNumber) (*SectorOnChainInfo, error)
	FindSector(abi.SectorNumber) (*SectorLocation, error)
	GetSectorExpiration(abi.SectorNumber) (*SectorExpiration, error)
	GetPrecommittedSector(abi.SectorNumber) (*SectorPreCommitOnChainInfo, error)
	ForEachPrecommittedSector(func(SectorPreCommitOnChainInfo) error) error
	LoadSectors(sectorNos *bitfield.BitField) ([]*SectorOnChainInfo, error)
	NumLiveSectors() (uint64, error)
	IsAllocated(abi.SectorNumber) (bool, error)
	// UnallocatedSectorNumbers returns up to count unallocated sector numbers (or less than
	// count if there aren't enough).
	UnallocatedSectorNumbers(count int) ([]abi.SectorNumber, error)
	GetAllocatedSectors() (*bitfield.BitField, error)

	// Note that ProvingPeriodStart is deprecated and will be renamed / removed in a future version of actors
	GetProvingPeriodStart() (abi.ChainEpoch, error)
	// Testing only
	EraseAllUnproven() error

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
	GetState() interface{}
}

type Deadline interface {
	LoadPartition(idx uint64) (Partition, error)
	ForEachPartition(cb func(idx uint64, part Partition) error) error
	PartitionsPoSted() (bitfield.BitField, error)

	PartitionsChanged(Deadline) (bool, error)
	DisputableProofCount() (uint64, error)
	DailyFee() (abi.TokenAmount, error)
}

type Partition interface {
	// AllSectors returns all sector numbers in this partition, including faulty, unproven, and terminated sectors
	AllSectors() (bitfield.BitField, error)

	// Subset of sectors detected/declared faulty and not yet recovered (excl. from PoSt).
	// Faults ∩ Terminated = ∅
	FaultySectors() (bitfield.BitField, error)

	// Subset of faulty sectors expected to recover on next PoSt
	// Recoveries ∩ Terminated = ∅
	RecoveringSectors() (bitfield.BitField, error)

	// Live sectors are those that are not terminated (but may be faulty).
	LiveSectors() (bitfield.BitField, error)

	// Active sectors are those that are neither terminated nor faulty nor unproven, i.e. actively contributing power.
	ActiveSectors() (bitfield.BitField, error)

	// Unproven sectors in this partition. This bitfield will be cleared on
	// a successful window post (or at the end of the partition's next
	// deadline). At that time, any still unproven sectors will be added to
	// the faulty sector bitfield.
	UnprovenSectors() (bitfield.BitField, error)
}

type SectorOnChainInfo = minertypes18.SectorOnChainInfo

func PreferredSealProofTypeFromWindowPoStType(nver network.Version, proof abi.RegisteredPoStProof, configWantSynthetic bool) (abi.RegisteredSealProof, error) {
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

	if nver < MinSyntheticPoRepVersion || !configWantSynthetic {
		switch proof {
		case abi.RegisteredPoStProof_StackedDrgWindow2KiBV1, abi.RegisteredPoStProof_StackedDrgWindow2KiBV1_1:
			return abi.RegisteredSealProof_StackedDrg2KiBV1_1, nil
		case abi.RegisteredPoStProof_StackedDrgWindow8MiBV1, abi.RegisteredPoStProof_StackedDrgWindow8MiBV1_1:
			return abi.RegisteredSealProof_StackedDrg8MiBV1_1, nil
		case abi.RegisteredPoStProof_StackedDrgWindow512MiBV1, abi.RegisteredPoStProof_StackedDrgWindow512MiBV1_1:
			return abi.RegisteredSealProof_StackedDrg512MiBV1_1, nil
		case abi.RegisteredPoStProof_StackedDrgWindow32GiBV1, abi.RegisteredPoStProof_StackedDrgWindow32GiBV1_1:
			return abi.RegisteredSealProof_StackedDrg32GiBV1_1, nil
		case abi.RegisteredPoStProof_StackedDrgWindow64GiBV1, abi.RegisteredPoStProof_StackedDrgWindow64GiBV1_1:
			return abi.RegisteredSealProof_StackedDrg64GiBV1_1, nil
		default:
			return -1, xerrors.Errorf("unrecognized window post type: %d", proof)
		}
	}

	switch proof {
	case abi.RegisteredPoStProof_StackedDrgWindow2KiBV1, abi.RegisteredPoStProof_StackedDrgWindow2KiBV1_1:
		return abi.RegisteredSealProof_StackedDrg2KiBV1_1_Feat_SyntheticPoRep, nil
	case abi.RegisteredPoStProof_StackedDrgWindow8MiBV1, abi.RegisteredPoStProof_StackedDrgWindow8MiBV1_1:
		return abi.RegisteredSealProof_StackedDrg8MiBV1_1_Feat_SyntheticPoRep, nil
	case abi.RegisteredPoStProof_StackedDrgWindow512MiBV1, abi.RegisteredPoStProof_StackedDrgWindow512MiBV1_1:
		return abi.RegisteredSealProof_StackedDrg512MiBV1_1_Feat_SyntheticPoRep, nil
	case abi.RegisteredPoStProof_StackedDrgWindow32GiBV1, abi.RegisteredPoStProof_StackedDrgWindow32GiBV1_1:
		return abi.RegisteredSealProof_StackedDrg32GiBV1_1_Feat_SyntheticPoRep, nil
	case abi.RegisteredPoStProof_StackedDrgWindow64GiBV1, abi.RegisteredPoStProof_StackedDrgWindow64GiBV1_1:
		return abi.RegisteredSealProof_StackedDrg64GiBV1_1_Feat_SyntheticPoRep, nil
	default:
		return -1, xerrors.Errorf("unrecognized window post type: %d", proof)
	}
}

func WinningPoStProofTypeFromWindowPoStProofType(nver network.Version, proof abi.RegisteredPoStProof) (abi.RegisteredPoStProof, error) {
	switch proof {
	case abi.RegisteredPoStProof_StackedDrgWindow2KiBV1, abi.RegisteredPoStProof_StackedDrgWindow2KiBV1_1:
		return abi.RegisteredPoStProof_StackedDrgWinning2KiBV1, nil
	case abi.RegisteredPoStProof_StackedDrgWindow8MiBV1, abi.RegisteredPoStProof_StackedDrgWindow8MiBV1_1:
		return abi.RegisteredPoStProof_StackedDrgWinning8MiBV1, nil
	case abi.RegisteredPoStProof_StackedDrgWindow512MiBV1, abi.RegisteredPoStProof_StackedDrgWindow512MiBV1_1:
		return abi.RegisteredPoStProof_StackedDrgWinning512MiBV1, nil
	case abi.RegisteredPoStProof_StackedDrgWindow32GiBV1, abi.RegisteredPoStProof_StackedDrgWindow32GiBV1_1:
		return abi.RegisteredPoStProof_StackedDrgWinning32GiBV1, nil
	case abi.RegisteredPoStProof_StackedDrgWindow64GiBV1, abi.RegisteredPoStProof_StackedDrgWindow64GiBV1_1:
		return abi.RegisteredPoStProof_StackedDrgWinning64GiBV1, nil
	default:
		return -1, xerrors.Errorf("unknown proof type %d", proof)
	}
}

type MinerInfo = minertypes.MinerInfo
type BeneficiaryTerm = minertypes.BeneficiaryTerm
type PendingBeneficiaryChange = minertypes.PendingBeneficiaryChange
type WorkerKeyChange = minertypes.WorkerKeyChange
type SectorPreCommitOnChainInfo = minertypes.SectorPreCommitOnChainInfo
type SectorPreCommitInfo = minertypes.SectorPreCommitInfo
type SubmitWindowedPoStParams = minertypes.SubmitWindowedPoStParams
type PoStPartition = minertypes.PoStPartition
type RecoveryDeclaration = minertypes.RecoveryDeclaration
type FaultDeclaration = minertypes.FaultDeclaration
type DeclareFaultsRecoveredParams = minertypes.DeclareFaultsRecoveredParams
type DeclareFaultsParams = minertypes.DeclareFaultsParams
type ProveCommitAggregateParams = minertypes.ProveCommitAggregateParams
type ProveCommitSectorParams = minertypes.ProveCommitSectorParams
type ProveReplicaUpdatesParams = minertypes.ProveReplicaUpdatesParams
type ProveReplicaUpdatesParams2 = minertypes.ProveReplicaUpdatesParams2
type ReplicaUpdate = minertypes.ReplicaUpdate
type ReplicaUpdate2 = minertypes.ReplicaUpdate2
type PreCommitSectorBatchParams = minertypes.PreCommitSectorBatchParams
type PreCommitSectorBatchParams2 = minertypes.PreCommitSectorBatchParams2
type ExtendSectorExpiration2Params = minertypes.ExtendSectorExpiration2Params
type SectorClaim = minertypes.SectorClaim
type ExpirationExtension2 = minertypes.ExpirationExtension2
type CompactPartitionsParams = minertypes.CompactPartitionsParams
type WithdrawBalanceParams = minertypes.WithdrawBalanceParams
type MaxTerminationFeeParams = minertypes18.MaxTerminationFeeParams
type MaxTerminationFeeReturn = minertypes18.MaxTerminationFeeReturn
type InitialPledgeReturn = minertypes18.InitialPledgeReturn

type PieceActivationManifest = minertypes13.PieceActivationManifest
type ProveCommitSectors3Params = minertypes13.ProveCommitSectors3Params
type SectorActivationManifest = minertypes13.SectorActivationManifest
type ProveReplicaUpdates3Params = minertypes13.ProveReplicaUpdates3Params
type SectorUpdateManifest = minertypes13.SectorUpdateManifest
type SectorOnChainInfoFlags = minertypes18.SectorOnChainInfoFlags
type VerifiedAllocationKey = minertypes13.VerifiedAllocationKey

var QAPowerMax = minertypes.QAPowerMax

type WindowPostVerifyInfo = proof.WindowPoStVerifyInfo

var WPoStProvingPeriod = func() abi.ChainEpoch { return minertypes.WPoStProvingPeriod }
var WPoStChallengeWindow = func() abi.ChainEpoch { return minertypes.WPoStChallengeWindow }

const WPoStPeriodDeadlines = minertypes.WPoStPeriodDeadlines
const WPoStChallengeLookback = minertypes.WPoStChallengeLookback
const FaultDeclarationCutoff = minertypes.FaultDeclarationCutoff
const MinAggregatedSectors = minertypes.MinAggregatedSectors
const MinSectorExpiration = minertypes.MinSectorExpiration

var TermFeePledgeMultiple = minertypes18.TermFeePledgeMultiple
var TermFeeMaxFaultFeeMultiple = minertypes18.TermFeeMaxFaultFeeMultiple

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

func AllCodes() []cid.Cid {
	return []cid.Cid{
		(&state0{}).Code(),
		(&state2{}).Code(),
		(&state3{}).Code(),
		(&state4{}).Code(),
		(&state5{}).Code(),
		(&state6{}).Code(),
		(&state7{}).Code(),
		(&state8{}).Code(),
		(&state9{}).Code(),
		(&state10{}).Code(),
		(&state11{}).Code(),
		(&state12{}).Code(),
		(&state13{}).Code(),
		(&state14{}).Code(),
		(&state15{}).Code(),
		(&state16{}).Code(),
		(&state17{}).Code(),
		(&state18{}).Code(),
	}
}

func PledgePenaltyForContinuedFault(
	nwVer network.Version,
	rewardEstimate builtin.FilterEstimate,
	networkQaPowerEstimate builtin.FilterEstimate,
	qaSectorPower abi.StoragePower,
) (abi.TokenAmount, error) {
	v, err := actorstypes.VersionForNetwork(nwVer)
	if err != nil {
		return big.Zero(), err
	}

	if v <= actorstypes.Version16 {
		return minertypes16.PledgePenaltyForContinuedFault(
			smoothing16.FilterEstimate{
				PositionEstimate: rewardEstimate.PositionEstimate,
				VelocityEstimate: rewardEstimate.VelocityEstimate,
			},
			smoothing16.FilterEstimate{
				PositionEstimate: networkQaPowerEstimate.PositionEstimate,
				VelocityEstimate: networkQaPowerEstimate.VelocityEstimate,
			},
			qaSectorPower,
		), nil
	}

	switch v {
	case actorstypes.Version17:
		return minertypes17.PledgePenaltyForContinuedFault(
			smoothing17.FilterEstimate{
				PositionEstimate: rewardEstimate.PositionEstimate,
				VelocityEstimate: rewardEstimate.VelocityEstimate,
			},
			smoothing17.FilterEstimate{
				PositionEstimate: networkQaPowerEstimate.PositionEstimate,
				VelocityEstimate: networkQaPowerEstimate.VelocityEstimate,
			},
			qaSectorPower,
		), nil
	case actorstypes.Version18:
		return minertypes18.PledgePenaltyForContinuedFault(
			smoothing18.FilterEstimate{
				PositionEstimate: rewardEstimate.PositionEstimate,
				VelocityEstimate: rewardEstimate.VelocityEstimate,
			},
			smoothing18.FilterEstimate{
				PositionEstimate: networkQaPowerEstimate.PositionEstimate,
				VelocityEstimate: networkQaPowerEstimate.VelocityEstimate,
			},
			qaSectorPower,
		), nil
	default:
		return big.Zero(), xerrors.Errorf("unsupported network version: %d", nwVer)
	}
}

func PledgePenaltyForTermination(
	nwVer network.Version,
	initialPledge abi.TokenAmount,
	sectorAge abi.ChainEpoch,
	faultFee abi.TokenAmount,
) (abi.TokenAmount, error) {
	v, err := actorstypes.VersionForNetwork(nwVer)
	if err != nil {
		return big.Zero(), err
	}

	switch v {
	case actorstypes.Version16:
		return minertypes16.PledgePenaltyForTermination(initialPledge, sectorAge, faultFee), nil
	case actorstypes.Version17:
		return minertypes17.PledgePenaltyForTermination(initialPledge, sectorAge, faultFee), nil
	case actorstypes.Version18:
		return minertypes18.PledgePenaltyForTermination(initialPledge, sectorAge, faultFee), nil
	default:
		return big.Zero(), xerrors.Errorf("unsupported network version: %d", nwVer)
	}
}

func ExpectedRewardForPower(
	nwVer network.Version,
	rewardEstimate, networkQAPowerEstimate builtin.FilterEstimate,
	qaSectorPower abi.StoragePower,
	projectionDuration abi.ChainEpoch,
) (abi.TokenAmount, error) {
	v, err := actorstypes.VersionForNetwork(nwVer)
	if err != nil {
		return big.Zero(), err
	}

	switch v {
	case actorstypes.Version13:
		return minertypes13.ExpectedRewardForPower(
			smoothing13.FilterEstimate{
				PositionEstimate: rewardEstimate.PositionEstimate,
				VelocityEstimate: rewardEstimate.VelocityEstimate,
			},
			smoothing13.FilterEstimate{
				PositionEstimate: networkQAPowerEstimate.PositionEstimate,
				VelocityEstimate: networkQAPowerEstimate.VelocityEstimate,
			},
			qaSectorPower,
			projectionDuration,
		), nil
	case actorstypes.Version14:
		return minertypes14.ExpectedRewardForPower(
			smoothing14.FilterEstimate{
				PositionEstimate: rewardEstimate.PositionEstimate,
				VelocityEstimate: rewardEstimate.VelocityEstimate,
			},
			smoothing14.FilterEstimate{
				PositionEstimate: networkQAPowerEstimate.PositionEstimate,
				VelocityEstimate: networkQAPowerEstimate.VelocityEstimate,
			},
			qaSectorPower,
			projectionDuration,
		), nil
	case actorstypes.Version15:
		return minertypes15.ExpectedRewardForPower(
			smoothing15.FilterEstimate{
				PositionEstimate: rewardEstimate.PositionEstimate,
				VelocityEstimate: rewardEstimate.VelocityEstimate,
			},
			smoothing15.FilterEstimate{
				PositionEstimate: networkQAPowerEstimate.PositionEstimate,
				VelocityEstimate: networkQAPowerEstimate.VelocityEstimate,
			},
			qaSectorPower,
			projectionDuration,
		), nil
	case actorstypes.Version16:
		return minertypes16.ExpectedRewardForPower(
			smoothing16.FilterEstimate{
				PositionEstimate: rewardEstimate.PositionEstimate,
				VelocityEstimate: rewardEstimate.VelocityEstimate,
			},
			smoothing16.FilterEstimate{
				PositionEstimate: networkQAPowerEstimate.PositionEstimate,
				VelocityEstimate: networkQAPowerEstimate.VelocityEstimate,
			},
			qaSectorPower,
			projectionDuration,
		), nil
	case actorstypes.Version17:
		return minertypes17.ExpectedRewardForPower(
			smoothing17.FilterEstimate{
				PositionEstimate: rewardEstimate.PositionEstimate,
				VelocityEstimate: rewardEstimate.VelocityEstimate,
			},
			smoothing17.FilterEstimate{
				PositionEstimate: networkQAPowerEstimate.PositionEstimate,
				VelocityEstimate: networkQAPowerEstimate.VelocityEstimate,
			},
			qaSectorPower,
			projectionDuration,
		), nil
	case actorstypes.Version18:
		return minertypes18.ExpectedRewardForPower(
			smoothing18.FilterEstimate{
				PositionEstimate: rewardEstimate.PositionEstimate,
				VelocityEstimate: rewardEstimate.VelocityEstimate,
			},
			smoothing18.FilterEstimate{
				PositionEstimate: networkQAPowerEstimate.PositionEstimate,
				VelocityEstimate: networkQAPowerEstimate.VelocityEstimate,
			},
			qaSectorPower,
			projectionDuration,
		), nil
	default:
		return big.Zero(), xerrors.Errorf("unsupported network version: %d", nwVer)
	}
}
