package lpmessage

import (
	"context"
	"os"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
)

var log = logging.Logger("lpmessage")

type SenderAPI interface {
	StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
	GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, tsk types.TipSetKey) (*types.Message, error)
	WalletBalance(ctx context.Context, addr address.Address) (big.Int, error)
	MpoolGetNonce(context.Context, address.Address) (uint64, error)
	MpoolPush(context.Context, *types.SignedMessage) (cid.Cid, error)
}

type SignerAPI interface {
	WalletSignMessage(context.Context, address.Address, *types.Message) (*types.SignedMessage, error)
}

// Sender abstracts away highly-available message sending with coordination through
// HarmonyDB. It make sure that nonces are assigned transactionally, and that
// messages are correctly broadcasted to the network. It is not a Task in the sense
// of a HarmonyTask interface, just a helper for tasks which need to send messages
// to the network.
type Sender struct {
	api    SenderAPI
	signer SignerAPI

	db     *harmonydb.DB
	noSend bool // For early testing with only 1 lotus-provider
}

// NewSender creates a new Sender.
func NewSender(api SenderAPI, signer SignerAPI, db *harmonydb.DB) *Sender {
	return &Sender{
		api:    api,
		signer: signer,
		db:     db,
		noSend: os.Getenv("LOTUS_PROVDER_NO_SEND") != "",
	}
}

// Send atomically assigns a nonce, signs, and pushes a message
// to mempool.
// maxFee is only used when GasFeeCap/GasPremium fields aren't specified
//
// When maxFee is set to 0, Send will guess appropriate fee
// based on current chain conditions
//
// Send behaves much like fullnodeApi.MpoolPushMessage, but it coordinates
// through HarmonyDB, making it safe to broadcast messages from multiple independent
// API nodes
//
// Send is also currently more strict about required parameters than MpoolPushMessage
func (s *Sender) Send(ctx context.Context, msg *types.Message, mss *api.MessageSendSpec, reason string) (cid.Cid, error) {
	if mss == nil {
		return cid.Undef, xerrors.Errorf("MessageSendSpec cannot be nil")
	}
	if (mss.MsgUuid != uuid.UUID{}) {
		return cid.Undef, xerrors.Errorf("MessageSendSpec.MsgUuid must be zero")
	}

	fromA, err := s.api.StateAccountKey(ctx, msg.From, types.EmptyTSK)
	if err != nil {
		return cid.Undef, xerrors.Errorf("getting key address: %w", err)
	}

	msg.From = fromA

	if msg.Nonce != 0 {
		return cid.Undef, xerrors.Errorf("Send expects message nonce to be 0, was %d", msg.Nonce)
	}

	msg, err = s.api.GasEstimateMessageGas(ctx, msg, mss, types.EmptyTSK)
	if err != nil {
		return cid.Undef, xerrors.Errorf("GasEstimateMessageGas error: %w", err)
	}

	b, err := s.api.WalletBalance(ctx, msg.From)
	if err != nil {
		return cid.Undef, xerrors.Errorf("mpool push: getting origin balance: %w", err)
	}

	requiredFunds := big.Add(msg.Value, msg.RequiredFunds())
	if b.LessThan(requiredFunds) {
		return cid.Undef, xerrors.Errorf("mpool push: not enough funds: %s < %s", b, requiredFunds)
	}

	var sigMsg *types.SignedMessage

	// start db tx
	c, err := s.db.BeginTransaction(ctx, func(tx *harmonydb.Tx) (commit bool, err error) {
		// assign nonce (max(api.MpoolGetNonce, db nonce+1))
		msgNonce, err := s.api.MpoolGetNonce(ctx, fromA)
		if err != nil {
			return false, xerrors.Errorf("getting nonce from mpool: %w", err)
		}

		// get nonce from db
		var dbNonce *uint64
		r := tx.QueryRow(`select max(nonce) from message_sends where from_key = $1`, fromA.String())
		if err := r.Scan(&dbNonce); err != nil {
			return false, xerrors.Errorf("getting nonce from db: %w", err)
		}

		if dbNonce != nil && *dbNonce+1 > msgNonce {
			msgNonce = *dbNonce + 1
		}

		msg.Nonce = msgNonce

		// sign message
		sigMsg, err = s.signer.WalletSignMessage(ctx, msg.From, msg)
		if err != nil {
			return false, xerrors.Errorf("signing message: %w", err)
		}

		data, err := sigMsg.Serialize()
		if err != nil {
			return false, xerrors.Errorf("serializing message: %w", err)
		}

		jsonBytes, err := sigMsg.MarshalJSON()
		if err != nil {
			return false, xerrors.Errorf("marshaling message: %w", err)
		}

		if s.noSend {
			log.Errorw("SKIPPED SENDING MESSAGE PER ENVIRONMENT VARIABLE - NOT PRODUCTION SAFE",
				"from_key", fromA.String(),
				"nonce", msg.Nonce,
				"to_addr", msg.To.String(),
				"signed_data", data,
				"signed_json", string(jsonBytes),
				"signed_cid", sigMsg.Cid(),
				"send_reason", reason,
			)
			return true, nil // nothing committed
		}
		// write to db
		c, err := tx.Exec(`insert into message_sends (from_key, nonce, to_addr, signed_data, signed_json, signed_cid, send_reason) values ($1, $2, $3, $4, $5, $6, $7)`,
			fromA.String(), msg.Nonce, msg.To.String(), data, string(jsonBytes), sigMsg.Cid().String(), reason)
		if err != nil {
			return false, xerrors.Errorf("inserting message into db: %w", err)
		}
		if c != 1 {
			return false, xerrors.Errorf("inserting message into db: expected 1 row to be affected, got %d", c)
		}

		// commit
		return true, nil
	})
	if err != nil || !c {
		return cid.Undef, xerrors.Errorf("transaction failed or didn't commit: %w", err)
	}
	if s.noSend {
		return sigMsg.Cid(), nil
	}

	// push to mpool
	_, err = s.api.MpoolPush(ctx, sigMsg)
	if err != nil {
		// TODO: We may get nonce gaps here..

		return cid.Undef, xerrors.Errorf("mpool push: failed to push message: %w", err)
	}

	// update db recocd to say it was pushed (set send_success to true)
	cn, err := s.db.Exec(ctx, `update message_sends set send_success = true where from_key = $1 and nonce = $2`, fromA.String(), msg.Nonce)
	if err != nil {
		return cid.Undef, xerrors.Errorf("updating db record: %w", err)
	}
	if cn != 1 {
		return cid.Undef, xerrors.Errorf("updating db record: expected 1 row to be affected, got %d", cn)
	}

	log.Infow("sent message", "cid", sigMsg.Cid(), "from", fromA, "to", msg.To, "nonce", msg.Nonce, "value", msg.Value, "gaslimit", msg.GasLimit)

	return sigMsg.Cid(), nil
}
