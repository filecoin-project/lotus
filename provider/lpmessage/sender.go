package lpmessage

import (
	"bytes"
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/lib/promise"
)

var log = logging.Logger("lpmessage")

var SendLockedWait = 100 * time.Millisecond

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
// messages are correctly broadcasted to the network. It ensures that messages
// are sent serially, and that failures to send don't cause nonce gaps.
type Sender struct {
	api SenderAPI

	sendTask *SendTask

	db *harmonydb.DB
}

type SendTask struct {
	sendTF promise.Promise[harmonytask.AddTaskFunc]

	api    SenderAPI
	signer SignerAPI

	db *harmonydb.DB
}

func (s *SendTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	ctx := context.TODO()

	// get message from db

	var dbMsg struct {
		FromKey string `db:"from_key"`
		ToAddr  string `db:"to_addr"`

		UnsignedData []byte `db:"unsigned_data"`
		UnsignedCid  string `db:"unsigned_cid"`

		// may not be null if we have somehow already signed but failed to send this message
		Nonce      *uint64 `db:"nonce"`
		SignedData []byte  `db:"signed_data"`
	}

	err = s.db.QueryRow(ctx, `
		SELECT from_key, nonce, to_addr, unsigned_data, unsigned_cid 
		FROM message_sends 
		WHERE send_task_id = $1`, taskID).Scan(
		&dbMsg.FromKey, &dbMsg.Nonce, &dbMsg.ToAddr, &dbMsg.UnsignedData, &dbMsg.UnsignedCid)
	if err != nil {
		return false, xerrors.Errorf("getting message from db: %w", err)
	}

	// deserialize the message
	var msg types.Message
	err = msg.UnmarshalCBOR(bytes.NewReader(dbMsg.UnsignedData))
	if err != nil {
		return false, xerrors.Errorf("unmarshaling unsigned db message: %w", err)
	}

	// get db send lock
	for {
		// check if we still own the task
		if !stillOwned() {
			return false, xerrors.Errorf("lost ownership of task")
		}

		// try to acquire lock
		cn, err := s.db.Exec(ctx, `
			INSERT INTO message_send_locks (from_key, task_id, claimed_at) 
			VALUES ($1, $2, CURRENT_TIMESTAMP) ON CONFLICT (from_key) DO UPDATE 
			SET task_id = EXCLUDED.task_id, claimed_at = CURRENT_TIMESTAMP 
			WHERE message_send_locks.task_id = $2;`, dbMsg.FromKey, taskID)
		if err != nil {
			return false, xerrors.Errorf("acquiring send lock: %w", err)
		}

		if cn == 1 {
			// we got the lock
			break
		}

		// we didn't get the lock, wait a bit and try again
		log.Infow("waiting for send lock", "task_id", taskID, "from", dbMsg.FromKey)
		time.Sleep(SendLockedWait)
	}

	// defer release db send lock
	defer func() {
		_, err2 := s.db.Exec(ctx, `
			DELETE from message_send_locks WHERE from_key = $1 AND task_id = $2`, dbMsg.FromKey, taskID)
		if err2 != nil {
			log.Errorw("releasing send lock", "task_id", taskID, "from", dbMsg.FromKey, "error", err2)

			// make sure harmony retries this task so that we eventually release this lock
			done = false
			err = multierr.Append(err, xerrors.Errorf("releasing send lock: %w", err2))
		}
	}()

	// assign nonce IF NOT ASSIGNED (max(api.MpoolGetNonce, db nonce+1))
	var sigMsg *types.SignedMessage

	if dbMsg.Nonce == nil {
		msgNonce, err := s.api.MpoolGetNonce(ctx, msg.From)
		if err != nil {
			return false, xerrors.Errorf("getting nonce from mpool: %w", err)
		}

		// get nonce from db
		var dbNonce *uint64
		r := s.db.QueryRow(ctx, `
			SELECT MAX(nonce) FROM message_sends WHERE from_key = $1 AND send_success = true`, msg.From.String())
		if err := r.Scan(&dbNonce); err != nil {
			return false, xerrors.Errorf("getting nonce from db: %w", err)
		}

		if dbNonce != nil && *dbNonce+1 > msgNonce {
			msgNonce = *dbNonce + 1
		}

		msg.Nonce = msgNonce

		// sign message
		sigMsg, err = s.signer.WalletSignMessage(ctx, msg.From, &msg)
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

		// write to db

		n, err := s.db.Exec(ctx, `
			UPDATE message_sends SET nonce = $1, signed_data = $2, signed_json = $3, signed_cid = $4 
			WHERE send_task_id = $5`,
			msg.Nonce, data, string(jsonBytes), sigMsg.Cid().String(), taskID)
		if err != nil {
			return false, xerrors.Errorf("updating db record: %w", err)
		}
		if n != 1 {
			log.Errorw("updating db record: expected 1 row to be affected, got %d", n)
			return false, xerrors.Errorf("updating db record: expected 1 row to be affected, got %d", n)
		}
	} else {
		// Note: this handles an unlikely edge-case:
		// We have previously signed the message but either failed to send it or failed to update the db
		// note that when that happens the likely cause is the provider process losing its db connection
		// or getting killed before it can update the db. In that case the message lock will still be held
		// so it will be safe to rebroadcast the signed message

		// deserialize the signed message
		sigMsg = new(types.SignedMessage)
		err = sigMsg.UnmarshalCBOR(bytes.NewReader(dbMsg.SignedData))
		if err != nil {
			return false, xerrors.Errorf("unmarshaling signed db message: %w", err)
		}
	}

	// send!
	_, err = s.api.MpoolPush(ctx, sigMsg)

	// persist send result
	var sendSuccess = err == nil
	var sendError string
	if err != nil {
		sendError = err.Error()
	}

	_, err = s.db.Exec(ctx, `
		UPDATE message_sends SET send_success = $1, send_error = $2, send_time = CURRENT_TIMESTAMP 
		WHERE send_task_id = $3`, sendSuccess, sendError, taskID)
	if err != nil {
		return false, xerrors.Errorf("updating db record: %w", err)
	}

	return true, nil
}

func (s *SendTask) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	if len(ids) == 0 {
		// probably can't happen, but panicking is bad
		return nil, nil
	}

	if s.signer == nil {
		// can't sign messages here
		return nil, nil
	}

	return &ids[0], nil
}

func (s *SendTask) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Max:  1024,
		Name: "SendMessage",
		Cost: resources.Resources{
			Cpu: 0,
			Gpu: 0,
			Ram: 1 << 20,
		},
		MaxFailures: 1000,
		Follows:     nil,
	}
}

func (s *SendTask) Adder(taskFunc harmonytask.AddTaskFunc) {
	s.sendTF.Set(taskFunc)
}

var _ harmonytask.TaskInterface = &SendTask{}

// NewSender creates a new Sender.
func NewSender(api SenderAPI, signer SignerAPI, db *harmonydb.DB) (*Sender, *SendTask) {
	st := &SendTask{
		api:    api,
		signer: signer,
		db:     db,
	}

	return &Sender{
		api: api,
		db:  db,

		sendTask: st,
	}, st
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

	// push the task
	taskAdder := s.sendTask.sendTF.Val(ctx)

	unsBytes := new(bytes.Buffer)
	err = msg.MarshalCBOR(unsBytes)
	if err != nil {
		return cid.Undef, xerrors.Errorf("marshaling message: %w", err)
	}

	var sendTaskID *harmonytask.TaskID
	taskAdder(func(id harmonytask.TaskID, tx *harmonydb.Tx) (shouldCommit bool, seriousError error) {
		_, err := tx.Exec(`insert into message_sends (from_key, to_addr, send_reason, unsigned_data, unsigned_cid, send_task_id) values ($1, $2, $3, $4, $5, $6)`,
			msg.From.String(), msg.To.String(), reason, unsBytes.Bytes(), msg.Cid().String(), id)
		if err != nil {
			return false, xerrors.Errorf("inserting message into db: %w", err)
		}

		sendTaskID = &id

		return true, nil
	})

	if sendTaskID == nil {
		return cid.Undef, xerrors.Errorf("failed to add task")
	}

	// wait for exec
	var (
		pollInterval    = 50 * time.Millisecond
		pollIntervalMul = 2
		maxPollInterval = 5 * time.Second
		pollLoops       = 0

		sigCid  cid.Cid
		sendErr error
	)

	for {
		var err error
		var sigCidStr, sendError *string
		var sendSuccess *bool

		err = s.db.QueryRow(ctx, `select signed_cid, send_success, send_error from message_sends where send_task_id = $1`, &sendTaskID).Scan(&sigCidStr, &sendSuccess, &sendError)
		if err != nil {
			return cid.Undef, xerrors.Errorf("getting cid for task: %w", err)
		}

		if sendSuccess == nil {
			time.Sleep(pollInterval)
			pollLoops++
			pollInterval *= time.Duration(pollIntervalMul)
			if pollInterval > maxPollInterval {
				pollInterval = maxPollInterval
			}

			continue
		}

		if sigCidStr == nil || sendError == nil {
			// should never happen because sendSuccess is already not null here
			return cid.Undef, xerrors.Errorf("got null values for sigCidStr or sendError, this should never happen")
		}

		if !*sendSuccess {
			sendErr = xerrors.Errorf("send error: %s", *sendError)
		} else {
			sigCid, err = cid.Parse(*sigCidStr)
			if err != nil {
				return cid.Undef, xerrors.Errorf("parsing signed cid: %w", err)
			}
		}

		break
	}

	log.Infow("sent message", "cid", sigCid, "task_id", taskAdder, "send_error", sendErr, "poll_loops", pollLoops)

	return sigCid, sendErr
}
