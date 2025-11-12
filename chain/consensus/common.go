package consensus

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/multiformats/go-varint"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opencensus.io/stats"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"
	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/api"
	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/lib/async"
	"github.com/filecoin-project/lotus/metrics"
)

// Common operations shared by all consensus algorithm implementations.
var log = logging.Logger("consensus-common")

// RunAsyncChecks accepts a list of checks to perform in parallel.
//
// Each consensus algorithm may choose to perform a set of different
// checks when a new blocks is received.
func RunAsyncChecks(ctx context.Context, await []async.ErrorFuture) error {
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

// CommonBlkChecks performed by all consensus implementations.
func CommonBlkChecks(ctx context.Context, sm *stmgr.StateManager, cs *store.ChainStore,
	b *types.FullBlock, baseTs *types.TipSet) []async.ErrorFuture {
	h := b.Header
	msgsCheck := async.Err(func() error {
		if b.Cid() == buildconstants.WhitelistedBlock {
			return nil
		}

		if err := checkBlockMessages(ctx, sm, cs, b, baseTs); err != nil {
			return xerrors.Errorf("block had invalid messages: %w", err)
		}
		return nil
	})

	baseFeeCheck := async.Err(func() error {
		baseFee, err := cs.ComputeBaseFee(ctx, baseTs)
		if err != nil {
			return xerrors.Errorf("computing base fee: %w", err)
		}
		if types.BigCmp(baseFee, b.Header.ParentBaseFee) != 0 {
			return xerrors.Errorf("base fee doesn't match: %s (header) != %s (computed)",
				b.Header.ParentBaseFee, baseFee)
		}
		return nil
	})

	stateRootCheck := async.Err(func() error {
		stateroot, precp, err := sm.TipSetState(ctx, baseTs)
		if err != nil {
			return xerrors.Errorf("get tipsetstate(%d, %s) failed: %w", h.Height, h.Parents, err)
		}

		if stateroot != h.ParentStateRoot {
			msgs, err := cs.MessagesForTipset(ctx, baseTs)
			if err != nil {
				log.Error("failed to load messages for tipset during tipset state mismatch error: ", err)
			} else {
				log.Warn("Messages for tipset with mismatching state:")
				for i, m := range msgs {
					mm := m.VMMessage()
					log.Warnf("Message[%d]: from=%s to=%s method=%d params=%x", i, mm.From, mm.To, mm.Method, mm.Params)
				}
			}

			return xerrors.Errorf("parent state root did not match computed state (%s != %s)", h.ParentStateRoot, stateroot)
		}

		if precp != h.ParentMessageReceipts {
			return xerrors.Errorf("parent receipts root did not match computed value (%s != %s)", precp, h.ParentMessageReceipts)
		}
		return nil
	})

	return []async.ErrorFuture{
		msgsCheck,
		baseFeeCheck,
		stateRootCheck,
	}
}

func IsValidEthTxForSending(nv network.Version, smsg *types.SignedMessage) bool {
	ethTx, err := ethtypes.EthTransactionFromSignedFilecoinMessage(smsg)
	if err != nil {
		return false
	}

	if nv < network.Version23 && ethTx.Type() != ethtypes.EIP1559TxType {
		return false
	}
	return true
}

func IsValidForSending(nv network.Version, act *types.Actor) bool {
	// Before nv18 (Hygge), we only supported built-in account actors as senders.
	//
	// Note: this gate is probably superfluous, since:
	// 1. Placeholder actors cannot be created before nv18.
	// 2. EthAccount actors cannot be created before nv18.
	// 3. Delegated addresses cannot be created before nv18.
	//
	// But it's a safeguard.
	//
	// Note 2: ad-hoc checks for network versions like this across the codebase
	// will be problematic with networks with diverging version lineages
	// (e.g. Hyperspace). We need to revisit this strategy entirely.
	if nv < network.Version18 {
		return builtin.IsAccountActor(act.Code)
	}

	// After nv18, we also support other kinds of senders.
	if builtin.IsAccountActor(act.Code) || builtin.IsEthAccountActor(act.Code) {
		return true
	}

	// Allow placeholder actors with a delegated address and nonce 0 to send a message.
	// These will be converted to an EthAccount actor on first send.
	if !builtin.IsPlaceholderActor(act.Code) || act.Nonce != 0 || act.DelegatedAddress == nil || act.DelegatedAddress.Protocol() != address.Delegated {
		return false
	}

	// Only allow such actors to send if their delegated address is in the EAM's namespace.
	id, _, err := varint.FromUvarint(act.DelegatedAddress.Payload())
	return err == nil && id == builtintypes.EthereumAddressManagerActorID
}

func checkBlockMessages(ctx context.Context, sm *stmgr.StateManager, cs *store.ChainStore, b *types.FullBlock, baseTs *types.TipSet) error {
	{
		var sigCids []cid.Cid // this is what we get for people not wanting the marshalcbor method on the cid type
		var pubks [][]byte

		for _, m := range b.BlsMessages {
			sigCids = append(sigCids, m.Cid())

			pubk, err := sm.GetBlsPublicKey(ctx, m.From, baseTs)
			if err != nil {
				return xerrors.Errorf("failed to load bls public to validate block: %w", err)
			}

			pubks = append(pubks, pubk)
		}

		if err := VerifyBlsAggregate(ctx, b.Header.BLSAggregate, sigCids, pubks); err != nil {
			return xerrors.Errorf("bls aggregate signature was invalid: %w", err)
		}
	}

	nonces := make(map[address.Address]uint64)

	stateroot, _, err := sm.TipSetState(ctx, baseTs)
	if err != nil {
		return xerrors.Errorf("failed to compute tipsettate for %s: %w", baseTs.Key(), err)
	}

	st, err := state.LoadStateTree(cs.ActorStore(ctx), stateroot)
	if err != nil {
		return xerrors.Errorf("failed to load base state tree: %w", err)
	}

	nv := sm.GetNetworkVersion(ctx, b.Header.Height)
	pl := vm.PricelistByEpoch(b.Header.Height)
	var sumGasLimit int64
	checkMsg := func(msg types.ChainMsg) error {
		m := msg.VMMessage()

		// Phase 1: syntactic validation, as defined in the spec
		minGas := pl.OnChainMessage(msg.ChainLength())
		if err := m.ValidForBlockInclusion(minGas.Total(), nv); err != nil {
			return xerrors.Errorf("msg %s invalid for block inclusion: %w", m.Cid(), err)
		}

		// ValidForBlockInclusion checks if any single message does not exceed BlockGasLimit
		// So below is overflow safe
		sumGasLimit += m.GasLimit
		if sumGasLimit > buildconstants.BlockGasLimit {
			return xerrors.Errorf("block gas limit exceeded")
		}

		// Phase 2: (Partial) semantic validation:
		// the sender exists and is an account actor, and the nonces make sense
		var sender address.Address
		if nv >= network.Version13 {
			sender, err = st.LookupIDAddress(m.From)
			if err != nil {
				return xerrors.Errorf("failed to lookup sender %s: %w", m.From, err)
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

			if !IsValidForSending(nv, act) {
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

		c, err := store.PutMessage(ctx, tmpbs, m)
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
		if nv >= network.Version14 && !IsValidSecpkSigType(nv, m.Signature.Type) {
			return xerrors.Errorf("block had invalid signed message at index %d: %w", i, err)
		}

		if m.Signature.Type == crypto.SigTypeDelegated && !IsValidEthTxForSending(nv, m) {
			return xerrors.Errorf("network version should be at least NV23 for sending legacy ETH transactions; but current network version is %d", nv)
		}

		if err := checkMsg(m); err != nil {
			return xerrors.Errorf("block had invalid secpk message at index %d: %w", i, err)
		}

		// `From` being an account actor is only validated inside the `vm.ResolveToDeterministicAddr` call
		// in `StateManager.ResolveToDeterministicAddress` here (and not in `checkMsg`).
		kaddr, err := sm.ResolveToDeterministicAddress(ctx, m.Message.From, baseTs)
		if err != nil {
			return xerrors.Errorf("failed to resolve key addr: %w", err)
		}

		if err := AuthenticateMessage(m, kaddr); err != nil {
			return xerrors.Errorf("failed to validate signature: %w", err)
		}

		c, err := store.PutMessage(ctx, tmpbs, m)
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
		return xerrors.Errorf("failed to root bls msgs: %w", err)

	}

	smroot, err := smArr.Root()
	if err != nil {
		return xerrors.Errorf("failed to root secp msgs: %w", err)
	}

	mrcid, err := tmpstore.Put(ctx, &types.MsgMeta{
		BlsMessages:   bmroot,
		SecpkMessages: smroot,
	})
	if err != nil {
		return xerrors.Errorf("failed to put msg meta: %w", err)
	}

	if b.Header.Messages != mrcid {
		return fmt.Errorf("messages didnt match message root in header")
	}

	// Finally, flush.
	err = vm.Copy(ctx, tmpbs, cs.ChainBlockstore(), mrcid)
	if err != nil {
		return xerrors.Errorf("failed to flush:%w", err)
	}

	return nil
}

// CreateBlockHeader generates the block header from the block template of
// the block being proposed.
func CreateBlockHeader(ctx context.Context, sm *stmgr.StateManager, pts *types.TipSet,
	bt *api.BlockTemplate) (*types.BlockHeader, []*types.Message, []*types.SignedMessage, error) {

	st, recpts, err := sm.TipSetState(ctx, pts)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("failed to load tipset state: %w", err)
	}
	next := &types.BlockHeader{
		Miner:         bt.Miner,
		Parents:       bt.Parents.Cids(),
		Ticket:        bt.Ticket,
		ElectionProof: bt.Eproof,

		BeaconEntries:         bt.BeaconValues,
		Height:                bt.Epoch,
		Timestamp:             bt.Timestamp,
		WinPoStProof:          bt.WinningPoStProof,
		ParentStateRoot:       st,
		ParentMessageReceipts: recpts,
	}

	var blsMessages []*types.Message
	var secpkMessages []*types.SignedMessage

	var blsMsgCids, secpkMsgCids []cid.Cid
	var blsSigs []crypto.Signature
	nv := sm.GetNetworkVersion(ctx, bt.Epoch)
	for _, msgTmp := range bt.Messages {
		msg := msgTmp
		if msg.Signature.Type == crypto.SigTypeBLS {
			blsSigs = append(blsSigs, msg.Signature)
			blsMessages = append(blsMessages, &msg.Message)

			c, err := sm.ChainStore().PutMessage(ctx, &msg.Message)
			if err != nil {
				return nil, nil, nil, err
			}

			blsMsgCids = append(blsMsgCids, c)
		} else if IsValidSecpkSigType(nv, msg.Signature.Type) {
			c, err := sm.ChainStore().PutMessage(ctx, msg)
			if err != nil {
				return nil, nil, nil, err
			}

			secpkMsgCids = append(secpkMsgCids, c)
			secpkMessages = append(secpkMessages, msg)

		} else {
			return nil, nil, nil, xerrors.Errorf("unknown sig type: %d", msg.Signature.Type)
		}
	}

	store := sm.ChainStore().ActorStore(ctx)
	blsmsgroot, err := ToMessagesArray(store, blsMsgCids)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("building bls amt: %w", err)
	}
	secpkmsgroot, err := ToMessagesArray(store, secpkMsgCids)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("building secpk amt: %w", err)
	}

	mmcid, err := store.Put(store.Context(), &types.MsgMeta{
		BlsMessages:   blsmsgroot,
		SecpkMessages: secpkmsgroot,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	next.Messages = mmcid

	aggSig, err := AggregateSignatures(blsSigs)
	if err != nil {
		return nil, nil, nil, err
	}

	next.BLSAggregate = aggSig
	pweight, err := sm.ChainStore().Weight(ctx, pts)
	if err != nil {
		return nil, nil, nil, err
	}
	next.ParentWeight = pweight

	baseFee, err := sm.ChainStore().ComputeBaseFee(ctx, pts)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("computing base fee: %w", err)
	}
	next.ParentBaseFee = baseFee

	return next, blsMessages, secpkMessages, err

}

// Basic sanity-checks performed when a block is proposed locally.
func validateLocalBlock(ctx context.Context, msg *pubsub.Message) (pubsub.ValidationResult, string) {
	stats.Record(ctx, metrics.BlockPublished.M(1))

	if size := msg.Size(); size > 1<<20-1<<15 {
		log.Errorf("ignoring oversize block (%dB)", size)
		return pubsub.ValidationIgnore, "oversize_block"
	}

	blk, what, err := decodeAndCheckBlock(msg)
	if err != nil {
		log.Errorf("got invalid local block: %s", err)
		return pubsub.ValidationIgnore, what
	}

	msg.ValidatorData = blk
	stats.Record(ctx, metrics.BlockValidationSuccess.M(1))
	return pubsub.ValidationAccept, ""
}

func decodeAndCheckBlock(msg *pubsub.Message) (*types.BlockMsg, string, error) {
	blk, err := types.DecodeBlockMsg(msg.GetData())
	if err != nil {
		return nil, "invalid", xerrors.Errorf("error decoding block: %w", err)
	}

	if count := len(blk.BlsMessages) + len(blk.SecpkMessages); count > buildconstants.BlockMessageLimit {
		return nil, "too_many_messages", fmt.Errorf("block contains too many messages (%d)", count)
	}

	// make sure we have a signature
	if blk.Header.BlockSig == nil {
		return nil, "missing_signature", fmt.Errorf("block without a signature")
	}

	return blk, "", nil
}

func validateMsgMeta(ctx context.Context, msg *types.BlockMsg) error {
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
