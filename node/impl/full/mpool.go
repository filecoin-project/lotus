package full

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/messagesigner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type MpoolModuleAPI interface {
	MpoolPush(ctx context.Context, smsg *types.SignedMessage) (cid.Cid, error)
}

var _ MpoolModuleAPI = *new(api.FullNode)

// MpoolModule provides a default implementation of MpoolModuleAPI.
// It can be swapped out with another implementation through Dependency
// Injection (for example with a thin RPC client).
type MpoolModule struct {
	fx.In

	Mpool *messagepool.MessagePool
}

var _ MpoolModuleAPI = (*MpoolModule)(nil)

type MpoolAPI struct {
	fx.In

	MpoolModuleAPI

	WalletAPI
	GasAPI

	RaftAPI

	MessageSigner messagesigner.MsgSigner

	PushLocks *dtypes.MpoolLocker
}

func (a *MpoolAPI) MpoolGetConfig(context.Context) (*types.MpoolConfig, error) {
	return a.Mpool.GetConfig(), nil
}

func (a *MpoolAPI) MpoolSetConfig(ctx context.Context, cfg *types.MpoolConfig) error {
	return a.Mpool.SetConfig(ctx, cfg)
}

func (a *MpoolAPI) MpoolSelect(ctx context.Context, tsk types.TipSetKey, ticketQuality float64) ([]*types.SignedMessage, error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	return a.Mpool.SelectMessages(ctx, ts, ticketQuality)
}

func (a *MpoolAPI) MpoolPending(ctx context.Context, tsk types.TipSetKey) ([]*types.SignedMessage, error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	pending, mpts := a.Mpool.Pending(ctx)

	haveCids := map[cid.Cid]struct{}{}
	for _, m := range pending {
		haveCids[m.Cid()] = struct{}{}
	}

	if ts == nil || mpts.Height() > ts.Height() {
		return pending, nil
	}

	for {
		if mpts.Height() == ts.Height() {
			if mpts.Equals(ts) {
				return pending, nil
			}

			// different blocks in tipsets of the same height
			// we exclude messages that have been included in blocks in the mpool tipset
			have, err := a.Mpool.MessagesForBlocks(ctx, mpts.Blocks())
			if err != nil {
				return nil, xerrors.Errorf("getting messages for base ts: %w", err)
			}

			for _, m := range have {
				haveCids[m.Cid()] = struct{}{}
			}
		}

		msgs, err := a.Mpool.MessagesForBlocks(ctx, ts.Blocks())
		if err != nil {
			return nil, xerrors.Errorf(": %w", err)
		}

		for _, m := range msgs {
			if _, ok := haveCids[m.Cid()]; ok {
				continue
			}

			haveCids[m.Cid()] = struct{}{}
			pending = append(pending, m)
		}

		if mpts.Height() >= ts.Height() {
			return pending, nil
		}

		ts, err = a.Chain.LoadTipSet(ctx, ts.Parents())
		if err != nil {
			return nil, xerrors.Errorf("loading parent tipset: %w", err)
		}
	}
}

func (a *MpoolAPI) MpoolClear(ctx context.Context, local bool) error {
	a.Mpool.Cleanup(ctx, api.MpoolCleanupOpts{DropLocal: local})
	return nil
}

func (a *MpoolAPI) MpoolCleanup(ctx context.Context, co api.MpoolCleanupOpts) error {
	a.Mpool.Cleanup(ctx, co)
	return nil
}

func (m *MpoolModule) MpoolPush(ctx context.Context, smsg *types.SignedMessage) (cid.Cid, error) {
	return m.Mpool.Push(ctx, smsg, true)
}

func (a *MpoolAPI) MpoolPushUntrusted(ctx context.Context, smsg *types.SignedMessage) (cid.Cid, error) {
	return a.Mpool.PushUntrusted(ctx, smsg)
}

func (a *MpoolAPI) MpoolPushMessage(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec) (*types.SignedMessage, error) {
	cp := *msg
	msg = &cp
	inMsg := *msg

	// Redirect to leader if current node is not leader. A single non raft based node is always the leader
	if !a.RaftAPI.IsLeader(ctx) {
		var signedMsg types.SignedMessage
		redirected, err := a.RaftAPI.RedirectToLeader(ctx, "MpoolPushMessage", api.MpoolMessageWhole{Msg: msg, Spec: spec}, &signedMsg)
		if err != nil {
			return nil, err
		}
		// It's possible that the current node became the leader between the check and the redirect
		// In that case, continue with rest of execution and only return signedMsg if something was redirected
		if redirected {
			return &signedMsg, nil
		}
	}

	// Generate spec and uuid if not available in the message
	if spec == nil {
		spec = &api.MessageSendSpec{
			MsgUuid: uuid.New(),
		}
	} else if (spec.MsgUuid == uuid.UUID{}) {
		spec.MsgUuid = uuid.New()
	} else {
		// Check if this uuid has already been processed. Ignore if uuid is not populated
		signedMessage, err := a.MessageSigner.GetSignedMessage(ctx, spec.MsgUuid)
		if err == nil {
			log.Warnf("Message already processed. cid=%s", signedMessage.Cid())
			return signedMessage, nil
		}
	}

	fromA, err := a.Stmgr.ResolveToDeterministicAddress(ctx, msg.From, nil)
	if err != nil {
		return nil, xerrors.Errorf("getting key address: %w", err)
	}
	{
		done, err := a.PushLocks.TakeLock(ctx, fromA)
		if err != nil {
			return nil, xerrors.Errorf("taking lock: %w", err)
		}
		defer done()
	}

	if msg.Nonce != 0 {
		return nil, xerrors.Errorf("MpoolPushMessage expects message nonce to be 0, was %d", msg.Nonce)
	}

	msg, err = a.GasAPI.GasEstimateMessageGas(ctx, msg, spec, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("GasEstimateMessageGas error: %w", err)
	}

	if msg.GasPremium.GreaterThan(msg.GasFeeCap) {
		inJson, _ := json.Marshal(inMsg)
		outJson, _ := json.Marshal(msg)
		return nil, xerrors.Errorf("After estimation, GasPremium is greater than GasFeeCap, inmsg: %s, outmsg: %s",
			inJson, outJson)
	}

	if msg.From.Protocol() == address.ID {
		log.Warnf("Push from ID address (%s), adjusting to %s", msg.From, fromA)
		msg.From = fromA
	}

	b, err := a.WalletBalance(ctx, msg.From)
	if err != nil {
		return nil, xerrors.Errorf("mpool push: getting origin balance: %w", err)
	}

	requiredFunds := big.Add(msg.Value, msg.RequiredFunds())
	if b.LessThan(requiredFunds) {
		return nil, xerrors.Errorf("mpool push: not enough funds: %s < %s", b, requiredFunds)
	}

	// Sign and push the message
	signedMsg, err := a.MessageSigner.SignMessage(ctx, msg, spec, func(smsg *types.SignedMessage) error {
		if _, err := a.MpoolModuleAPI.MpoolPush(ctx, smsg); err != nil {
			return xerrors.Errorf("mpool push: failed to push message: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Store uuid->signed message in datastore
	err = a.MessageSigner.StoreSignedMessage(ctx, spec.MsgUuid, signedMsg)
	if err != nil {
		return nil, err
	}

	return signedMsg, nil
}

func (a *MpoolAPI) MpoolBatchPush(ctx context.Context, smsgs []*types.SignedMessage) ([]cid.Cid, error) {
	var messageCids []cid.Cid
	for _, smsg := range smsgs {
		smsgCid, err := a.Mpool.Push(ctx, smsg, true)
		if err != nil {
			return messageCids, err
		}
		messageCids = append(messageCids, smsgCid)
	}
	return messageCids, nil
}

func (a *MpoolAPI) MpoolBatchPushUntrusted(ctx context.Context, smsgs []*types.SignedMessage) ([]cid.Cid, error) {
	var messageCids []cid.Cid
	for _, smsg := range smsgs {
		smsgCid, err := a.Mpool.PushUntrusted(ctx, smsg)
		if err != nil {
			return messageCids, err
		}
		messageCids = append(messageCids, smsgCid)
	}
	return messageCids, nil
}

func (a *MpoolAPI) MpoolBatchPushMessage(ctx context.Context, msgs []*types.Message, spec *api.MessageSendSpec) ([]*types.SignedMessage, error) {
	var smsgs []*types.SignedMessage
	for _, msg := range msgs {
		smsg, err := a.MpoolPushMessage(ctx, msg, spec)
		if err != nil {
			return smsgs, err
		}
		smsgs = append(smsgs, smsg)
	}
	return smsgs, nil
}

func (a *MpoolAPI) MpoolCheckMessages(ctx context.Context, protos []*api.MessagePrototype) ([][]api.MessageCheckStatus, error) {
	return a.Mpool.CheckMessages(ctx, protos)
}

func (a *MpoolAPI) MpoolCheckPendingMessages(ctx context.Context, from address.Address) ([][]api.MessageCheckStatus, error) {
	return a.Mpool.CheckPendingMessages(ctx, from)
}

func (a *MpoolAPI) MpoolCheckReplaceMessages(ctx context.Context, msgs []*types.Message) ([][]api.MessageCheckStatus, error) {
	return a.Mpool.CheckReplaceMessages(ctx, msgs)
}

func (a *MpoolAPI) MpoolGetNonce(ctx context.Context, addr address.Address) (uint64, error) {
	return a.Mpool.GetNonce(ctx, addr, types.EmptyTSK)
}

func (a *MpoolAPI) MpoolSub(ctx context.Context) (<-chan api.MpoolUpdate, error) {
	return a.Mpool.Updates(ctx)
}
