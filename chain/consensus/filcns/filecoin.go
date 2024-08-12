package filcns

import (
	"bytes"
	"context"
	"errors"
	"os"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/specs-actors/v7/actors/runtime/proof"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/proofs"
	"github.com/filecoin-project/lotus/chain/rand"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/lib/async"
	"github.com/filecoin-project/lotus/lib/sigs"
)

var log = logging.Logger("fil-consensus")

type FilecoinEC struct {
	// The interface for accessing and putting tipsets into local storage
	store *store.ChainStore

	// handle to the random beacon for verification
	beacon beacon.Schedule

	// the state manager handles making state queries
	sm *stmgr.StateManager

	verifier proofs.Verifier

	genesis *types.TipSet
}

// Blocks that are more than MaxHeightDrift epochs above
// the theoretical max height based on systime are quickly rejected
const MaxHeightDrift = 5

var RewardFunc = func(ctx context.Context, vmi vm.Interface, em stmgr.ExecMonitor,
	epoch abi.ChainEpoch, ts *types.TipSet, params *reward.AwardBlockRewardParams) error {
	ser, err := actors.SerializeParams(params)
	if err != nil {
		return xerrors.Errorf("failed to serialize award params: %w", err)
	}
	rwMsg := &types.Message{
		From:       builtin.SystemActorAddr,
		To:         reward.Address,
		Nonce:      uint64(epoch),
		Value:      types.NewInt(0),
		GasFeeCap:  types.NewInt(0),
		GasPremium: types.NewInt(0),
		GasLimit:   1 << 30,
		Method:     reward.Methods.AwardBlockReward,
		Params:     ser,
	}
	ret, actErr := vmi.ApplyImplicitMessage(ctx, rwMsg)
	if actErr != nil {
		return xerrors.Errorf("failed to apply reward message: %w", actErr)
	}

	if !ret.ExitCode.IsSuccess() {
		return xerrors.Errorf("reward actor failed with exit code %d: %w", ret.ExitCode, ret.ActorErr)
	}

	if em != nil {
		if err := em.MessageApplied(ctx, ts, rwMsg.Cid(), rwMsg, ret, true); err != nil {
			return xerrors.Errorf("callback failed on reward message: %w", err)
		}
	}

	return nil
}

func NewFilecoinExpectedConsensus(sm *stmgr.StateManager, beacon beacon.Schedule, verifier proofs.Verifier, genesis chain.Genesis) consensus.Consensus {
	if build.InsecurePoStValidation {
		log.Warn("*********************************************************************************************")
		log.Warn(" [INSECURE-POST-VALIDATION] Insecure test validation is enabled. If you see this outside of a test, it is a severe bug! ")
		log.Warn("*********************************************************************************************")
	}

	return &FilecoinEC{
		store:    sm.ChainStore(),
		beacon:   beacon,
		sm:       sm,
		verifier: verifier,
		genesis:  genesis,
	}
}

func (filec *FilecoinEC) ValidateBlock(ctx context.Context, b *types.FullBlock) (err error) {
	if err := blockSanityChecks(b.Header); err != nil {
		return xerrors.Errorf("incoming header failed basic sanity checks: %w", err)
	}

	h := b.Header

	baseTs, err := filec.store.LoadTipSet(ctx, types.NewTipSetKey(h.Parents...))
	if err != nil {
		return xerrors.Errorf("load parent tipset failed (%s): %w", h.Parents, err)
	}

	winPoStNv := filec.sm.GetNetworkVersion(ctx, baseTs.Height())

	lbts, lbst, err := stmgr.GetLookbackTipSetForRound(ctx, filec.sm, baseTs, h.Height)
	if err != nil {
		return xerrors.Errorf("failed to get lookback tipset for block: %w", err)
	}

	// TODO: Optimization: See https://github.com/filecoin-project/lotus/issues/11597
	prevBeacon, err := filec.store.GetLatestBeaconEntry(ctx, baseTs)
	if err != nil {
		return xerrors.Errorf("failed to get latest beacon entry: %w", err)
	}

	// fast checks first
	if h.Height <= baseTs.Height() {
		return xerrors.Errorf("block height not greater than parent height: %d != %d", h.Height, baseTs.Height())
	}

	nulls := h.Height - (baseTs.Height() + 1)
	if tgtTs := baseTs.MinTimestamp() + buildconstants.BlockDelaySecs*uint64(nulls+1); h.Timestamp != tgtTs {
		return xerrors.Errorf("block has wrong timestamp: %d != %d", h.Timestamp, tgtTs)
	}

	now := uint64(build.Clock.Now().Unix())
	if h.Timestamp > now+buildconstants.AllowableClockDriftSecs {
		return xerrors.Errorf("block was from the future (now=%d, blk=%d): %w", now, h.Timestamp, consensus.ErrTemporal)
	}
	if h.Timestamp > now {
		log.Warnf("Got block from the future, but within threshold (%d > %d)", h.Timestamp, now)
	}

	minerCheck := async.Err(func() error {
		if err := filec.minerIsValid(ctx, h.Miner, baseTs); err != nil {
			return xerrors.Errorf("minerIsValid failed: %w", err)
		}
		return nil
	})

	pweight, err := filec.store.Weight(ctx, baseTs)
	if err != nil {
		return xerrors.Errorf("getting parent weight: %w", err)
	}

	if types.BigCmp(pweight, b.Header.ParentWeight) != 0 {
		return xerrors.Errorf("parent weight different: %s (header) != %s (computed)",
			b.Header.ParentWeight, pweight)
	}

	// Stuff that needs worker address
	waddr, err := stmgr.GetMinerWorkerRaw(ctx, filec.sm, lbst, h.Miner)
	if err != nil {
		return xerrors.Errorf("GetMinerWorkerRaw failed: %w", err)
	}

	winnerCheck := async.Err(func() error {
		if h.ElectionProof.WinCount < 1 {
			return xerrors.Errorf("block is not claiming to be a winner")
		}

		eligible, err := stmgr.MinerEligibleToMine(ctx, filec.sm, h.Miner, baseTs, lbts)
		if err != nil {
			return xerrors.Errorf("determining if miner has min power failed: %w", err)
		}

		if !eligible {
			return xerrors.New("block's miner is ineligible to mine")
		}

		rBeacon := *prevBeacon
		if len(h.BeaconEntries) != 0 {
			rBeacon = h.BeaconEntries[len(h.BeaconEntries)-1]
		}
		buf := new(bytes.Buffer)
		if err := h.Miner.MarshalCBOR(buf); err != nil {
			return xerrors.Errorf("failed to marshal miner address to cbor: %w", err)
		}

		vrfBase, err := rand.DrawRandomnessFromBase(rBeacon.Data, crypto.DomainSeparationTag_ElectionProofProduction, h.Height, buf.Bytes())
		if err != nil {
			return xerrors.Errorf("could not draw randomness: %w", err)
		}

		if err := VerifyElectionPoStVRF(ctx, waddr, vrfBase, h.ElectionProof.VRFProof); err != nil {
			return xerrors.Errorf("validating block election proof failed: %w", err)
		}

		slashed, err := stmgr.GetMinerSlashed(ctx, filec.sm, baseTs, h.Miner)
		if err != nil {
			return xerrors.Errorf("failed to check if block miner was slashed: %w", err)
		}

		if slashed {
			return xerrors.Errorf("received block was from slashed or invalid miner")
		}

		mpow, tpow, _, err := stmgr.GetPowerRaw(ctx, filec.sm, lbst, h.Miner)
		if err != nil {
			return xerrors.Errorf("failed getting power: %w", err)
		}

		j := h.ElectionProof.ComputeWinCount(mpow.QualityAdjPower, tpow.QualityAdjPower)
		if h.ElectionProof.WinCount != j {
			return xerrors.Errorf("miner claims wrong number of wins: miner: %d, computed: %d", h.ElectionProof.WinCount, j)
		}

		return nil
	})

	blockSigCheck := async.Err(func() error {
		if err := verifyBlockSignature(ctx, h, waddr); err != nil {
			return xerrors.Errorf("check block signature failed: %w", err)
		}
		return nil

	})

	beaconValuesCheck := async.Err(func() error {
		if os.Getenv("LOTUS_IGNORE_DRAND") == "_yes_" {
			return nil
		}

		nv := filec.sm.GetNetworkVersion(ctx, h.Height)
		if err := beacon.ValidateBlockValues(filec.beacon, nv, h, baseTs.Height(), *prevBeacon); err != nil {
			return xerrors.Errorf("failed to validate blocks random beacon values: %w", err)
		}
		return nil
	})

	tktsCheck := async.Err(func() error {
		buf := new(bytes.Buffer)
		if err := h.Miner.MarshalCBOR(buf); err != nil {
			return xerrors.Errorf("failed to marshal miner address to cbor: %w", err)
		}

		if h.Height > buildconstants.UpgradeSmokeHeight {
			buf.Write(baseTs.MinTicket().VRFProof)
		}

		beaconBase := *prevBeacon
		if len(h.BeaconEntries) != 0 {
			beaconBase = h.BeaconEntries[len(h.BeaconEntries)-1]
		}

		vrfBase, err := rand.DrawRandomnessFromBase(beaconBase.Data, crypto.DomainSeparationTag_TicketProduction, h.Height-buildconstants.TicketRandomnessLookback, buf.Bytes())
		if err != nil {
			return xerrors.Errorf("failed to compute vrf base for ticket: %w", err)
		}

		err = VerifyElectionPoStVRF(ctx, waddr, vrfBase, h.Ticket.VRFProof)
		if err != nil {
			return xerrors.Errorf("validating block tickets failed: %w", err)
		}
		return nil
	})

	wproofCheck := async.Err(func() error {
		if err := filec.VerifyWinningPoStProof(ctx, winPoStNv, h, *prevBeacon, lbst, waddr); err != nil {
			return xerrors.Errorf("invalid election post: %w", err)
		}
		return nil
	})

	commonChecks := consensus.CommonBlkChecks(ctx, filec.sm, filec.store, b, baseTs)
	await := append([]async.ErrorFuture{
		minerCheck,
		tktsCheck,
		blockSigCheck,
		beaconValuesCheck,
		wproofCheck,
		winnerCheck,
	}, commonChecks...)

	return consensus.RunAsyncChecks(ctx, await)
}

func blockSanityChecks(h *types.BlockHeader) error {
	if h.ElectionProof == nil {
		return xerrors.Errorf("block cannot have nil election proof")
	}

	if h.Ticket == nil {
		return xerrors.Errorf("block cannot have nil ticket")
	}

	if h.BlockSig == nil {
		return xerrors.Errorf("block had nil signature")
	}

	if h.BLSAggregate == nil {
		return xerrors.Errorf("block had nil bls aggregate signature")
	}

	if h.Miner.Protocol() != address.ID {
		return xerrors.Errorf("block had non-ID miner address")
	}

	return nil
}

func (filec *FilecoinEC) VerifyWinningPoStProof(ctx context.Context, nv network.Version, h *types.BlockHeader, prevBeacon types.BeaconEntry, lbst cid.Cid, waddr address.Address) error {
	if build.InsecurePoStValidation {
		if len(h.WinPoStProof) == 0 {
			return xerrors.Errorf("[INSECURE-POST-VALIDATION] No winning post proof given")
		}

		if string(h.WinPoStProof[0].ProofBytes) == "valid proof" {
			return nil
		}
		return xerrors.Errorf("[INSECURE-POST-VALIDATION] winning post was invalid")
	}

	buf := new(bytes.Buffer)
	if err := h.Miner.MarshalCBOR(buf); err != nil {
		return xerrors.Errorf("failed to marshal miner address: %w", err)
	}

	rbase := prevBeacon
	if len(h.BeaconEntries) > 0 {
		rbase = h.BeaconEntries[len(h.BeaconEntries)-1]
	}

	rand, err := rand.DrawRandomnessFromBase(rbase.Data, crypto.DomainSeparationTag_WinningPoStChallengeSeed, h.Height, buf.Bytes())
	if err != nil {
		return xerrors.Errorf("failed to get randomness for verifying winning post proof: %w", err)
	}

	mid, err := address.IDFromAddress(h.Miner)
	if err != nil {
		return xerrors.Errorf("failed to get ID from miner address %s: %w", h.Miner, err)
	}

	xsectors, err := stmgr.GetSectorsForWinningPoSt(ctx, nv, filec.verifier, filec.sm, lbst, h.Miner, rand)
	if err != nil {
		return xerrors.Errorf("getting winning post sector set: %w", err)
	}

	sectors := make([]proof.SectorInfo, len(xsectors))
	for i, xsi := range xsectors {
		sectors[i] = proof.SectorInfo{
			SealProof:    xsi.SealProof,
			SectorNumber: xsi.SectorNumber,
			SealedCID:    xsi.SealedCID,
		}
	}

	ok, err := filec.verifier.VerifyWinningPoSt(ctx, proof.WinningPoStVerifyInfo{
		Randomness:        rand,
		Proofs:            h.WinPoStProof,
		ChallengedSectors: sectors,
		Prover:            abi.ActorID(mid),
	})
	if err != nil {
		return xerrors.Errorf("failed to verify election post: %w", err)
	}

	if !ok {
		log.Errorf("invalid winning post (block: %s, %x; %v)", h.Cid(), rand, sectors)
		return xerrors.Errorf("winning post was invalid")
	}

	return nil
}

func (filec *FilecoinEC) IsEpochInConsensusRange(epoch abi.ChainEpoch) bool {
	if filec.genesis == nil {
		return true
	}

	// Don't try to sync anything before finality. Don't propagate such blocks either.
	//
	// We use _our_ current head, not the expected head, because the network's head can lag on
	// catch-up (after a network outage).
	if epoch < filec.store.GetHeaviestTipSet().Height()-policy.ChainFinality {
		return false
	}

	now := uint64(build.Clock.Now().Unix())
	return epoch <= (abi.ChainEpoch((now-filec.genesis.MinTimestamp())/buildconstants.BlockDelaySecs) + MaxHeightDrift)
}

func (filec *FilecoinEC) minerIsValid(ctx context.Context, maddr address.Address, baseTs *types.TipSet) error {
	act, err := filec.sm.LoadActor(ctx, power.Address, baseTs)
	if err != nil {
		return xerrors.Errorf("failed to load power actor: %w", err)
	}

	powState, err := power.Load(filec.store.ActorStore(ctx), act)
	if err != nil {
		return xerrors.Errorf("failed to load power actor state: %w", err)
	}

	_, exist, err := powState.MinerPower(maddr)
	if err != nil {
		return xerrors.Errorf("failed to look up miner's claim: %w", err)
	}

	if !exist {
		return xerrors.New("miner isn't valid")
	}

	return nil
}

func VerifyElectionPoStVRF(ctx context.Context, worker address.Address, rand []byte, evrf []byte) error {
	return VerifyVRF(ctx, worker, rand, evrf)
}

func VerifyVRF(ctx context.Context, worker address.Address, vrfBase, vrfproof []byte) error {
	_, span := trace.StartSpan(ctx, "VerifyVRF")
	defer span.End()

	sig := &crypto.Signature{
		Type: crypto.SigTypeBLS,
		Data: vrfproof,
	}

	if err := sigs.Verify(sig, worker, vrfBase); err != nil {
		return xerrors.Errorf("vrf was invalid: %w", err)
	}

	return nil
}

var ErrSoftFailure = errors.New("soft validation failure")
var ErrInsufficientPower = errors.New("incoming block's miner does not have minimum power")

func (filec *FilecoinEC) ValidateBlockHeader(ctx context.Context, b *types.BlockHeader) (rejectReason string, err error) {

	// we want to ensure that it is a block from a known miner; we reject blocks from unknown miners
	// to prevent spam attacks.
	// the logic works as follows: we lookup the miner in the chain for its key.
	// if we can find it then it's a known miner and we can validate the signature.
	// if we can't find it, we check whether we are (near) synced in the chain.
	// if we are not synced we cannot validate the block and we must ignore it.
	// if we are synced and the miner is unknown, then the block is rejcected.
	key, err := filec.checkPowerAndGetWorkerKey(ctx, b)
	if err != nil {
		if err != ErrSoftFailure && filec.isChainNearSynced() {
			log.Warnf("received block from unknown miner or miner that doesn't meet min power over pubsub; rejecting message")
			return "unknown_miner", err
		}

		log.Warnf("cannot validate block message; unknown miner or miner that doesn't meet min power in unsynced chain: %s", b.Cid())
		return "", err // ignore
	}

	if b.ElectionProof.WinCount < 1 {
		log.Errorf("block is not claiming to be winning")
		return "not_winning", xerrors.Errorf("block not winning")
	}

	err = sigs.CheckBlockSignature(ctx, b, key)
	if err != nil {
		log.Errorf("block signature verification failed: %s", err)
		return "signature_verification_failed", err
	}

	return "", nil
}

func (filec *FilecoinEC) checkPowerAndGetWorkerKey(ctx context.Context, bh *types.BlockHeader) (address.Address, error) {
	// we check that the miner met the minimum power at the lookback tipset

	baseTs := filec.store.GetHeaviestTipSet()
	lbts, lbst, err := stmgr.GetLookbackTipSetForRound(ctx, filec.sm, baseTs, bh.Height)
	if err != nil {
		log.Warnf("failed to load lookback tipset for incoming block: %s", err)
		return address.Undef, ErrSoftFailure
	}

	key, err := stmgr.GetMinerWorkerRaw(ctx, filec.sm, lbst, bh.Miner)
	if err != nil {
		log.Warnf("failed to resolve worker key for miner %s and block height %d: %s", bh.Miner, bh.Height, err)
		return address.Undef, ErrSoftFailure
	}

	// NOTE: we check to see if the miner was eligible in the lookback
	// tipset - 1 for historical reasons. DO NOT use the lookback state
	// returned by GetLookbackTipSetForRound.

	eligible, err := stmgr.MinerEligibleToMine(ctx, filec.sm, bh.Miner, baseTs, lbts)
	if err != nil {
		log.Warnf("failed to determine if incoming block's miner has minimum power: %s", err)
		return address.Undef, ErrSoftFailure
	}

	if !eligible {
		log.Warnf("incoming block's miner is ineligible")
		return address.Undef, ErrInsufficientPower
	}

	return key, nil
}

func (filec *FilecoinEC) isChainNearSynced() bool {
	ts := filec.store.GetHeaviestTipSet()
	timestamp := ts.MinTimestamp()
	timestampTime := time.Unix(int64(timestamp), 0)
	return build.Clock.Since(timestampTime) < 6*time.Hour
}

func verifyBlockSignature(ctx context.Context, h *types.BlockHeader,
	addr address.Address) error {
	return sigs.CheckBlockSignature(ctx, h, addr)
}

func signBlock(ctx context.Context, w api.Wallet,
	addr address.Address, next *types.BlockHeader) error {

	nosigbytes, err := next.SigningBytes()
	if err != nil {
		return xerrors.Errorf("failed to get signing bytes for block: %w", err)
	}

	sig, err := w.WalletSign(ctx, addr, nosigbytes, api.MsgMeta{
		Type: api.MTBlock,
	})
	if err != nil {
		return xerrors.Errorf("failed to sign new block: %w", err)
	}
	next.BlockSig = sig
	return nil
}

var _ consensus.Consensus = &FilecoinEC{}
