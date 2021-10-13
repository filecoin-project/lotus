package filcns

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/filecoin-project/lotus/chain/rand"

	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opencensus.io/stats"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"
	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"
	proof2 "github.com/filecoin-project/specs-actors/v2/actors/runtime/proof"

	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/lib/async"
	"github.com/filecoin-project/lotus/lib/sigs"
	"github.com/filecoin-project/lotus/metrics"
)

var log = logging.Logger("fil-consensus")

type FilecoinEC struct {
	// The interface for accessing and putting tipsets into local storage
	store *store.ChainStore

	// handle to the random beacon for verification
	beacon beacon.Schedule

	// the state manager handles making state queries
	sm *stmgr.StateManager

	verifier ffiwrapper.Verifier

	genesis *types.TipSet
}

// Blocks that are more than MaxHeightDrift epochs above
// the theoretical max height based on systime are quickly rejected
const MaxHeightDrift = 5

func NewFilecoinExpectedConsensus(sm *stmgr.StateManager, beacon beacon.Schedule, verifier ffiwrapper.Verifier, genesis chain.Genesis) consensus.Consensus {
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

	baseTs, err := filec.store.LoadTipSet(types.NewTipSetKey(h.Parents...))
	if err != nil {
		return xerrors.Errorf("load parent tipset failed (%s): %w", h.Parents, err)
	}

	winPoStNv := filec.sm.GetNtwkVersion(ctx, baseTs.Height())

	lbts, lbst, err := stmgr.GetLookbackTipSetForRound(ctx, filec.sm, baseTs, h.Height)
	if err != nil {
		return xerrors.Errorf("failed to get lookback tipset for block: %w", err)
	}

	prevBeacon, err := filec.store.GetLatestBeaconEntry(baseTs)
	if err != nil {
		return xerrors.Errorf("failed to get latest beacon entry: %w", err)
	}

	// fast checks first
	if h.Height <= baseTs.Height() {
		return xerrors.Errorf("block height not greater than parent height: %d != %d", h.Height, baseTs.Height())
	}

	nulls := h.Height - (baseTs.Height() + 1)
	if tgtTs := baseTs.MinTimestamp() + build.BlockDelaySecs*uint64(nulls+1); h.Timestamp != tgtTs {
		return xerrors.Errorf("block has wrong timestamp: %d != %d", h.Timestamp, tgtTs)
	}

	now := uint64(build.Clock.Now().Unix())
	if h.Timestamp > now+build.AllowableClockDriftSecs {
		return xerrors.Errorf("block was from the future (now=%d, blk=%d): %w", now, h.Timestamp, consensus.ErrTemporal)
	}
	if h.Timestamp > now {
		log.Warn("Got block from the future, but within threshold", h.Timestamp, build.Clock.Now().Unix())
	}

	msgsCheck := async.Err(func() error {
		if b.Cid() == build.WhitelistedBlock {
			return nil
		}

		if err := filec.checkBlockMessages(ctx, b, baseTs); err != nil {
			return xerrors.Errorf("block had invalid messages: %w", err)
		}
		return nil
	})

	minerCheck := async.Err(func() error {
		if err := filec.minerIsValid(ctx, h.Miner, baseTs); err != nil {
			return xerrors.Errorf("minerIsValid failed: %w", err)
		}
		return nil
	})

	baseFeeCheck := async.Err(func() error {
		baseFee, err := filec.store.ComputeBaseFee(ctx, baseTs)
		if err != nil {
			return xerrors.Errorf("computing base fee: %w", err)
		}
		if types.BigCmp(baseFee, b.Header.ParentBaseFee) != 0 {
			return xerrors.Errorf("base fee doesn't match: %s (header) != %s (computed)",
				b.Header.ParentBaseFee, baseFee)
		}
		return nil
	})
	pweight, err := filec.store.Weight(ctx, baseTs)
	if err != nil {
		return xerrors.Errorf("getting parent weight: %w", err)
	}

	if types.BigCmp(pweight, b.Header.ParentWeight) != 0 {
		return xerrors.Errorf("parrent weight different: %s (header) != %s (computed)",
			b.Header.ParentWeight, pweight)
	}

	stateRootCheck := async.Err(func() error {
		stateroot, precp, err := filec.sm.TipSetState(ctx, baseTs)
		if err != nil {
			return xerrors.Errorf("get tipsetstate(%d, %s) failed: %w", h.Height, h.Parents, err)
		}

		if stateroot != h.ParentStateRoot {
			msgs, err := filec.store.MessagesForTipset(baseTs)
			if err != nil {
				log.Error("failed to load messages for tipset during tipset state mismatch error: ", err)
			} else {
				log.Warn("Messages for tipset with mismatching state:")
				for i, m := range msgs {
					mm := m.VMMessage()
					log.Warnf("Message[%d]: from=%s to=%s method=%d params=%x", i, mm.From, mm.To, mm.Method, mm.Params)
				}
			}

			return xerrors.Errorf("parent state root did not match computed state (%s != %s)", stateroot, h.ParentStateRoot)
		}

		if precp != h.ParentMessageReceipts {
			return xerrors.Errorf("parent receipts root did not match computed value (%s != %s)", precp, h.ParentMessageReceipts)
		}

		return nil
	})

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

		vrfBase, err := rand.DrawRandomness(rBeacon.Data, crypto.DomainSeparationTag_ElectionProofProduction, h.Height, buf.Bytes())
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
		if err := sigs.CheckBlockSignature(ctx, h, waddr); err != nil {
			return xerrors.Errorf("check block signature failed: %w", err)
		}
		return nil
	})

	beaconValuesCheck := async.Err(func() error {
		if os.Getenv("LOTUS_IGNORE_DRAND") == "_yes_" {
			return nil
		}

		if err := beacon.ValidateBlockValues(filec.beacon, h, baseTs.Height(), *prevBeacon); err != nil {
			return xerrors.Errorf("failed to validate blocks random beacon values: %w", err)
		}
		return nil
	})

	tktsCheck := async.Err(func() error {
		buf := new(bytes.Buffer)
		if err := h.Miner.MarshalCBOR(buf); err != nil {
			return xerrors.Errorf("failed to marshal miner address to cbor: %w", err)
		}

		if h.Height > build.UpgradeSmokeHeight {
			buf.Write(baseTs.MinTicket().VRFProof)
		}

		beaconBase := *prevBeacon
		if len(h.BeaconEntries) != 0 {
			beaconBase = h.BeaconEntries[len(h.BeaconEntries)-1]
		}

		vrfBase, err := rand.DrawRandomness(beaconBase.Data, crypto.DomainSeparationTag_TicketProduction, h.Height-build.TicketRandomnessLookback, buf.Bytes())
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

	await := []async.ErrorFuture{
		minerCheck,
		tktsCheck,
		blockSigCheck,
		beaconValuesCheck,
		wproofCheck,
		winnerCheck,
		msgsCheck,
		baseFeeCheck,
		stateRootCheck,
	}

	var merr error
	for _, fut := range await {
		if err := fut.AwaitContext(ctx); err != nil {
			merr = multierror.Append(merr, err)
		}
	}
	if merr != nil {
		mulErr := merr.(*multierror.Error)
		mulErr.ErrorFormat = func(es []error) string {
			if len(es) == 1 {
				return fmt.Sprintf("1 error occurred:\n\t* %+v\n\n", es[0])
			}

			points := make([]string, len(es))
			for i, err := range es {
				points[i] = fmt.Sprintf("* %+v", err)
			}

			return fmt.Sprintf(
				"%d errors occurred:\n\t%s\n\n",
				len(es), strings.Join(points, "\n\t"))
		}
		return mulErr
	}

	return nil
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

	rand, err := rand.DrawRandomness(rbase.Data, crypto.DomainSeparationTag_WinningPoStChallengeSeed, h.Height, buf.Bytes())
	if err != nil {
		return xerrors.Errorf("failed to get randomness for verifying winning post proof: %w", err)
	}

	mid, err := address.IDFromAddress(h.Miner)
	if err != nil {
		return xerrors.Errorf("failed to get ID from miner address %s: %w", h.Miner, err)
	}

	sectors, err := stmgr.GetSectorsForWinningPoSt(ctx, nv, filec.verifier, filec.sm, lbst, h.Miner, rand)
	if err != nil {
		return xerrors.Errorf("getting winning post sector set: %w", err)
	}

	ok, err := ffiwrapper.ProofVerifier.VerifyWinningPoSt(ctx, proof2.WinningPoStVerifyInfo{
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

// TODO: We should extract this somewhere else and make the message pool and miner use the same logic
func (filec *FilecoinEC) checkBlockMessages(ctx context.Context, b *types.FullBlock, baseTs *types.TipSet) error {
	{
		var sigCids []cid.Cid // this is what we get for people not wanting the marshalcbor method on the cid type
		var pubks [][]byte

		for _, m := range b.BlsMessages {
			sigCids = append(sigCids, m.Cid())

			pubk, err := filec.sm.GetBlsPublicKey(ctx, m.From, baseTs)
			if err != nil {
				return xerrors.Errorf("failed to load bls public to validate block: %w", err)
			}

			pubks = append(pubks, pubk)
		}

		if err := consensus.VerifyBlsAggregate(ctx, b.Header.BLSAggregate, sigCids, pubks); err != nil {
			return xerrors.Errorf("bls aggregate signature was invalid: %w", err)
		}
	}

	nonces := make(map[address.Address]uint64)

	stateroot, _, err := filec.sm.TipSetState(ctx, baseTs)
	if err != nil {
		return err
	}

	st, err := state.LoadStateTree(filec.store.ActorStore(ctx), stateroot)
	if err != nil {
		return xerrors.Errorf("failed to load base state tree: %w", err)
	}

	nv := filec.sm.GetNtwkVersion(ctx, b.Header.Height)
	pl := vm.PricelistByEpoch(baseTs.Height())
	var sumGasLimit int64
	checkMsg := func(msg types.ChainMsg) error {
		m := msg.VMMessage()

		// Phase 1: syntactic validation, as defined in the spec
		minGas := pl.OnChainMessage(msg.ChainLength())
		if err := m.ValidForBlockInclusion(minGas.Total(), nv); err != nil {
			return err
		}

		// ValidForBlockInclusion checks if any single message does not exceed BlockGasLimit
		// So below is overflow safe
		sumGasLimit += m.GasLimit
		if sumGasLimit > build.BlockGasLimit {
			return xerrors.Errorf("block gas limit exceeded")
		}

		// Phase 2: (Partial) semantic validation:
		// the sender exists and is an account actor, and the nonces make sense
		var sender address.Address
		if filec.sm.GetNtwkVersion(ctx, b.Header.Height) >= network.Version13 {
			sender, err = st.LookupID(m.From)
			if err != nil {
				return err
			}
		} else {
			sender = m.From
		}

		if _, ok := nonces[sender]; !ok {
			// `GetActor` does not validate that this is an account actor.
			act, err := st.GetActor(sender)
			if err != nil {
				return xerrors.Errorf("failed to get actor: %w", err)
			}

			if !builtin.IsAccountActor(act.Code) {
				return xerrors.New("Sender must be an account actor")
			}
			nonces[sender] = act.Nonce
		}

		if nonces[sender] != m.Nonce {
			return xerrors.Errorf("wrong nonce (exp: %d, got: %d)", nonces[sender], m.Nonce)
		}
		nonces[sender]++

		return nil
	}

	// Validate message arrays in a temporary blockstore.
	tmpbs := bstore.NewMemory()
	tmpstore := blockadt.WrapStore(ctx, cbor.NewCborStore(tmpbs))

	bmArr := blockadt.MakeEmptyArray(tmpstore)
	for i, m := range b.BlsMessages {
		if err := checkMsg(m); err != nil {
			return xerrors.Errorf("block had invalid bls message at index %d: %w", i, err)
		}

		c, err := store.PutMessage(tmpbs, m)
		if err != nil {
			return xerrors.Errorf("failed to store message %s: %w", m.Cid(), err)
		}

		k := cbg.CborCid(c)
		if err := bmArr.Set(uint64(i), &k); err != nil {
			return xerrors.Errorf("failed to put bls message at index %d: %w", i, err)
		}
	}

	smArr := blockadt.MakeEmptyArray(tmpstore)
	for i, m := range b.SecpkMessages {
		if filec.sm.GetNtwkVersion(ctx, b.Header.Height) >= network.Version14 {
			if m.Signature.Type != crypto.SigTypeSecp256k1 {
				return xerrors.Errorf("block had invalid secpk message at index %d: %w", i, err)
			}
		}

		if err := checkMsg(m); err != nil {
			return xerrors.Errorf("block had invalid secpk message at index %d: %w", i, err)
		}

		// `From` being an account actor is only validated inside the `vm.ResolveToKeyAddr` call
		// in `StateManager.ResolveToKeyAddress` here (and not in `checkMsg`).
		kaddr, err := filec.sm.ResolveToKeyAddress(ctx, m.Message.From, baseTs)
		if err != nil {
			return xerrors.Errorf("failed to resolve key addr: %w", err)
		}

		if err := sigs.Verify(&m.Signature, kaddr, m.Message.Cid().Bytes()); err != nil {
			return xerrors.Errorf("secpk message %s has invalid signature: %w", m.Cid(), err)
		}

		c, err := store.PutMessage(tmpbs, m)
		if err != nil {
			return xerrors.Errorf("failed to store message %s: %w", m.Cid(), err)
		}
		k := cbg.CborCid(c)
		if err := smArr.Set(uint64(i), &k); err != nil {
			return xerrors.Errorf("failed to put secpk message at index %d: %w", i, err)
		}
	}

	bmroot, err := bmArr.Root()
	if err != nil {
		return err
	}

	smroot, err := smArr.Root()
	if err != nil {
		return err
	}

	mrcid, err := tmpstore.Put(ctx, &types.MsgMeta{
		BlsMessages:   bmroot,
		SecpkMessages: smroot,
	})
	if err != nil {
		return err
	}

	if b.Header.Messages != mrcid {
		return fmt.Errorf("messages didnt match message root in header")
	}

	// Finally, flush.
	return vm.Copy(ctx, tmpbs, filec.store.ChainBlockstore(), mrcid)
}

func (filec *FilecoinEC) IsEpochBeyondCurrMax(epoch abi.ChainEpoch) bool {
	if filec.genesis == nil {
		return false
	}

	now := uint64(build.Clock.Now().Unix())
	return epoch > (abi.ChainEpoch((now-filec.genesis.MinTimestamp())/build.BlockDelaySecs) + MaxHeightDrift)
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

func (filec *FilecoinEC) ValidateBlockPubsub(ctx context.Context, self bool, msg *pubsub.Message) (pubsub.ValidationResult, string) {
	if self {
		return filec.validateLocalBlock(ctx, msg)
	}

	// track validation time
	begin := build.Clock.Now()
	defer func() {
		log.Debugf("block validation time: %s", build.Clock.Since(begin))
	}()

	stats.Record(ctx, metrics.BlockReceived.M(1))

	recordFailureFlagPeer := func(what string) {
		// bv.Validate will flag the peer in that case
		panic(what)
	}

	blk, what, err := filec.decodeAndCheckBlock(msg)
	if err != nil {
		log.Error("got invalid block over pubsub: ", err)
		recordFailureFlagPeer(what)
		return pubsub.ValidationReject, what
	}

	// validate the block meta: the Message CID in the header must match the included messages
	err = filec.validateMsgMeta(ctx, blk)
	if err != nil {
		log.Warnf("error validating message metadata: %s", err)
		recordFailureFlagPeer("invalid_block_meta")
		return pubsub.ValidationReject, "invalid_block_meta"
	}

	reject, err := filec.validateBlockHeader(ctx, blk.Header)
	if err != nil {
		if reject == "" {
			log.Warn("ignoring block msg: ", err)
			return pubsub.ValidationIgnore, reject
		}
		recordFailureFlagPeer(reject)
		return pubsub.ValidationReject, reject
	}

	// all good, accept the block
	msg.ValidatorData = blk
	stats.Record(ctx, metrics.BlockValidationSuccess.M(1))
	return pubsub.ValidationAccept, ""
}

func (filec *FilecoinEC) validateLocalBlock(ctx context.Context, msg *pubsub.Message) (pubsub.ValidationResult, string) {
	stats.Record(ctx, metrics.BlockPublished.M(1))

	if size := msg.Size(); size > 1<<20-1<<15 {
		log.Errorf("ignoring oversize block (%dB)", size)
		return pubsub.ValidationIgnore, "oversize_block"
	}

	blk, what, err := filec.decodeAndCheckBlock(msg)
	if err != nil {
		log.Errorf("got invalid local block: %s", err)
		return pubsub.ValidationIgnore, what
	}

	msg.ValidatorData = blk
	stats.Record(ctx, metrics.BlockValidationSuccess.M(1))
	return pubsub.ValidationAccept, ""
}

func (filec *FilecoinEC) decodeAndCheckBlock(msg *pubsub.Message) (*types.BlockMsg, string, error) {
	blk, err := types.DecodeBlockMsg(msg.GetData())
	if err != nil {
		return nil, "invalid", xerrors.Errorf("error decoding block: %w", err)
	}

	if count := len(blk.BlsMessages) + len(blk.SecpkMessages); count > build.BlockMessageLimit {
		return nil, "too_many_messages", fmt.Errorf("block contains too many messages (%d)", count)
	}

	// make sure we have a signature
	if blk.Header.BlockSig == nil {
		return nil, "missing_signature", fmt.Errorf("block without a signature")
	}

	return blk, "", nil
}

func (filec *FilecoinEC) validateMsgMeta(ctx context.Context, msg *types.BlockMsg) error {
	// TODO there has to be a simpler way to do this without the blockstore dance
	// block headers use adt0
	store := blockadt.WrapStore(ctx, cbor.NewCborStore(bstore.NewMemory()))
	bmArr := blockadt.MakeEmptyArray(store)
	smArr := blockadt.MakeEmptyArray(store)

	for i, m := range msg.BlsMessages {
		c := cbg.CborCid(m)
		if err := bmArr.Set(uint64(i), &c); err != nil {
			return err
		}
	}

	for i, m := range msg.SecpkMessages {
		c := cbg.CborCid(m)
		if err := smArr.Set(uint64(i), &c); err != nil {
			return err
		}
	}

	bmroot, err := bmArr.Root()
	if err != nil {
		return err
	}

	smroot, err := smArr.Root()
	if err != nil {
		return err
	}

	mrcid, err := store.Put(store.Context(), &types.MsgMeta{
		BlsMessages:   bmroot,
		SecpkMessages: smroot,
	})

	if err != nil {
		return err
	}

	if msg.Header.Messages != mrcid {
		return fmt.Errorf("messages didn't match root cid in header")
	}

	return nil
}

func (filec *FilecoinEC) validateBlockHeader(ctx context.Context, b *types.BlockHeader) (rejectReason string, err error) {

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

var _ consensus.Consensus = &FilecoinEC{}
