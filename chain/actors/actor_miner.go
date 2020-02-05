package actors

import (
	"encoding/binary"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
)

type StorageMinerActorState struct {
	// PreCommittedSectors is the set of sectors that have been committed to but not
	// yet had their proofs submitted
	PreCommittedSectors map[string]*PreCommittedSector

	// All sectors this miner has committed.
	//
	// AMT[sectorID]ffi.PublicSectorInfo
	Sectors cid.Cid

	// TODO: Spec says 'StagedCommittedSectors', which one is it?

	// Sectors this miner is currently mining. It is only updated
	// when a PoSt is submitted (not as each new sector commitment is added).
	//
	// AMT[sectorID]ffi.PublicSectorInfo
	ProvingSet cid.Cid

	// TODO: these:
	//    SectorTable
	//    SectorExpirationQueue
	//    ChallengeStatus

	// Contains mostly static info about this miner
	Info cid.Cid

	// Faulty sectors reported since last SubmitPost
	FaultSet types.BitField

	LastFaultSubmission uint64

	// Amount of power this miner has.
	Power types.BigInt

	// Active is set to true after the miner has submitted their first PoSt
	Active bool

	// The height at which this miner was slashed at.
	SlashedAt uint64

	ElectionPeriodStart uint64
}

type MinerInfo struct {
	// Account that owns this miner.
	// - Income and returned collateral are paid to this address.
	// - This address is also allowed to change the worker address for the miner.
	Owner address.Address

	// Worker account for this miner.
	// This will be the key that is used to sign blocks created by this miner, and
	// sign messages sent on behalf of this miner to commit sectors, submit PoSts, and
	// other day to day miner activities.
	Worker address.Address

	// Libp2p identity that should be used when connecting to this miner.
	PeerID peer.ID

	// Amount of space in each sector committed to the network by this miner.
	SectorSize uint64

	// SubsectorCount
}

type PreCommittedSector struct {
	Info          SectorPreCommitInfo
	ReceivedEpoch uint64
}

type StorageMinerConstructorParams struct {
	Owner      address.Address
	Worker     address.Address
	SectorSize uint64
	PeerID     peer.ID
}

type SectorPreCommitInfo struct {
	SectorNumber uint64

	CommR     []byte // TODO: Spec says CID
	SealEpoch uint64
	DealIDs   []uint64
}

type maMethods struct {
	Constructor          uint64
	PreCommitSector      uint64
	ProveCommitSector    uint64
	SubmitFallbackPoSt   uint64
	SlashStorageFault    uint64
	GetCurrentProvingSet uint64
	ArbitrateDeal        uint64
	DePledge             uint64
	GetOwner             uint64
	GetWorkerAddr        uint64
	GetPower             uint64
	GetPeerID            uint64
	GetSectorSize        uint64
	UpdatePeerID         uint64
	ChangeWorker         uint64
	IsSlashed            uint64
	CheckMiner           uint64
	DeclareFaults        uint64
	SlashConsensusFault  uint64
	SubmitElectionPoSt   uint64
}

var MAMethods = maMethods{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}

func loadState(vmctx types.VMContext) (cid.Cid, *StorageMinerActorState, ActorError) {
	var self StorageMinerActorState
	oldstate := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(oldstate, &self); err != nil {
		return cid.Undef, nil, err
	}

	return oldstate, &self, nil
}

func loadMinerInfo(vmctx types.VMContext, m *StorageMinerActorState) (*MinerInfo, ActorError) {
	var mi MinerInfo
	if err := vmctx.Storage().Get(m.Info, &mi); err != nil {
		return nil, err
	}

	return &mi, nil
}

func uintToStringKey(i uint64) string {
	buf := make([]byte, 10)
	n := binary.PutUvarint(buf, i)
	return string(buf[:n])
}

type SectorProveCommitInfo struct {
	Proof    []byte
	SectorID uint64
	DealIDs  []uint64
}

func truncateHexPrint(b []byte) string {
	s := fmt.Sprintf("%x", b)
	if len(s) > 60 {
		return s[:20] + "..." + s[len(s)-20:]
	}
	return s
}

type SubmitFallbackPoStParams struct {
	Proof      []byte
	Candidates []types.EPostTicket
}

func CollateralForPower(power types.BigInt) types.BigInt {
	return types.BigMul(power, types.NewInt(10))
	/* TODO: this
	availableFil = FakeGlobalMethods.GetAvailableFil()
	totalNetworkPower = StorageMinerActor.GetTotalStorage()
	numMiners = StorageMarket.GetMinerCount()
	powerCollateral = availableFil * NetworkConstants.POWER_COLLATERAL_PROPORTION * power / totalNetworkPower
	perCapitaCollateral = availableFil * NetworkConstants.PER_CAPITA_COLLATERAL_PROPORTION / numMiners
	collateralRequired = math.Ceil(minerPowerCollateral + minerPerCapitaCollateral)
	return collateralRequired
	*/
}

type UpdatePeerIDParams struct {
	PeerID peer.ID
}

func isLate(height uint64, self *StorageMinerActorState) bool {
	return self.ElectionPeriodStart > 0 && height >= self.ElectionPeriodStart+build.SlashablePowerDelay
}

type CheckMinerParams struct {
	NetworkPower types.BigInt
}

type DeclareFaultsParams struct {
	Faults types.BitField
}

type MinerSlashConsensusFault struct {
	Slasher           address.Address
	AtHeight          uint64
	SlashedCollateral types.BigInt
}

func slasherShare(total types.BigInt, elapsed uint64) types.BigInt {
	// [int(pow(1.26, n) * 10) for n in range(30)]
	fracs := []uint64{10, 12, 15, 20, 25, 31, 40, 50, 63, 80, 100, 127, 160, 201, 254, 320, 403, 508, 640, 807, 1017, 1281, 1614, 2034, 2563, 3230, 4070, 5128, 6462, 8142}
	const precision = 10000

	var frac uint64
	if elapsed >= uint64(len(fracs)) {
		return total
	} else {
		frac = fracs[elapsed]
	}

	return types.BigDiv(
		types.BigMul(
			types.NewInt(frac),
			total,
		),
		types.NewInt(precision),
	)
}
