package index

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"

	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	amt4 "github.com/filecoin-project/go-amt-ipld/v4"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"

	// TODO: Solve this import cycle
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"
)

var logger = logging.Logger("chain/index")

type ChainIndexer struct {
	db *sql.DB
	cs *store.ChainStore

	stmts *preparedStatements

	resolver func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool)
}

type executedMessage struct {
	msg types.ChainMsg
	rct *types.MessageReceipt
	// events extracted from receipt
	evs []*types.Event
}

func (ci *ChainIndexer) loadExecutedMessages(ctx context.Context, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
	msgs, err := ci.cs.MessagesForTipset(ctx, msgTs)
	if err != nil {
		return nil, xerrors.Errorf("read messages: %w", err)
	}

	st := ci.cs.ActorStore(ctx)

	arr, err := blockadt.AsArray(st, rctTs.Blocks()[0].ParentMessageReceipts)
	if err != nil {
		return nil, xerrors.Errorf("load receipts amt: %w", err)
	}

	if uint64(len(msgs)) != arr.Length() {
		return nil, xerrors.Errorf("mismatching message and receipt counts (%d msgs, %d rcts)", len(msgs), arr.Length())
	}

	ems := make([]executedMessage, len(msgs))

	for i := 0; i < len(msgs); i++ {
		ems[i].msg = msgs[i]

		var rct types.MessageReceipt
		found, err := arr.Get(uint64(i), &rct)
		if err != nil {
			return nil, xerrors.Errorf("load receipt: %w", err)
		}
		if !found {
			return nil, xerrors.Errorf("receipt %d not found", i)
		}
		ems[i].rct = &rct

		if rct.EventsRoot == nil {
			continue
		}

		evtArr, err := amt4.LoadAMT(ctx, st, *rct.EventsRoot, amt4.UseTreeBitWidth(types.EventAMTBitwidth))
		if err != nil {
			return nil, xerrors.Errorf("load events amt: %w", err)
		}

		ems[i].evs = make([]*types.Event, evtArr.Len())
		var evt types.Event
		err = evtArr.ForEach(ctx, func(u uint64, deferred *cbg.Deferred) error {
			if u > math.MaxInt {
				return xerrors.Errorf("too many events")
			}
			if err := evt.UnmarshalCBOR(bytes.NewReader(deferred.Raw)); err != nil {
				return err
			}

			cpy := evt
			ems[i].evs[int(u)] = &cpy //nolint:scopelint
			return nil
		})

		if err != nil {
			return nil, xerrors.Errorf("read events: %w", err)
		}

	}

	return ems, nil
}

func (ci *ChainIndexer) GetMsgInfo(ctx context.Context, msg_cid cid.Cid) (MsgInfo, error) {
	var tipsetKeyCid []byte
	var height int64
	// query the tipset_messages table for the given message_cid using the `selectNonRevertedMessage` statement
	if err := ci.stmts.selectNonRevertedMessage.QueryRow(msg_cid.Bytes()).Scan(&tipsetKeyCid, &height); err != nil {
		return MsgInfo{}, xerrors.Errorf("error querying tipset_messages table: %w", err)
	}

	ts, err := cid.Cast(tipsetKeyCid)
	if err != nil {
		return MsgInfo{}, xerrors.Errorf("error casting tipsetKeyCid to cid: %w", err)
	}

	m := MsgInfo{
		Message: msg_cid,
		TipSet:  ts,
		Epoch:   abi.ChainEpoch(height),
	}

	return m, nil
}

// TODO: Also need to index eth tx hash <> msg cid from Mpool
func (ci *ChainIndexer) GetEthTxHashFromMsgCid(c cid.Cid) (ethtypes.EthHash, error) {
	row := ci.stmts.selectEthTxHash.QueryRow(c.Bytes())
	var hashString string
	err := row.Scan(&hashString)
	if err != nil {
		if err == sql.ErrNoRows {
			return ethtypes.EmptyEthHash, ErrNotFound
		}
		return ethtypes.EmptyEthHash, err
	}
	return ethtypes.ParseEthHash(hashString)
}

type eventFilter struct {
	minHeight abi.ChainEpoch // minimum epoch to apply filter or -1 if no minimum
	maxHeight abi.ChainEpoch // maximum epoch to apply filter or -1 if no maximum
	tipsetCid cid.Cid
	addresses []address.Address // list of actor addresses that are extpected to emit the event

	keysWithCodec map[string][]types.ActorEventBlock // map of key names to a list of alternate values that may match
}

func makePrefillFilterQuery(f *eventFilter, excludeReverted bool) ([]any, string) {
	clauses := []string{}
	values := []any{}
	joins := []string{}

	if f.tipsetCid != cid.Undef {
		clauses = append(clauses, "tm.tipset_key_cid=?")
		values = append(values, f.tipsetCid.Bytes())
	} else {
		if f.minHeight >= 0 && f.minHeight == f.maxHeight {
			clauses = append(clauses, "t.height=?")
			values = append(values, f.minHeight)
		} else {
			if f.maxHeight >= 0 && f.minHeight >= 0 {
				clauses = append(clauses, "t.height BETWEEN ? AND ?")
				values = append(values, f.minHeight, f.maxHeight)
			} else if f.minHeight >= 0 {
				clauses = append(clauses, "t.height >= ?")
				values = append(values, f.minHeight)
			} else if f.maxHeight >= 0 {
				clauses = append(clauses, "t.height <= ?")
				values = append(values, f.maxHeight)
			}
		}
	}

	if excludeReverted {
		clauses = append(clauses, "t.reverted=?")
		values = append(values, false)
	}

	if len(f.addresses) > 0 {
		for _, addr := range f.addresses {
			values = append(values, addr.Bytes())
		}
		clauses = append(clauses, "e.emitter_addr IN ("+strings.Repeat("?,", len(f.addresses)-1)+"?)")
	}

	if len(f.keysWithCodec) > 0 {
		join := 0
		for key, vals := range f.keysWithCodec {
			if len(vals) > 0 {
				join++
				joinAlias := fmt.Sprintf("ee%d", join)
				joins = append(joins, fmt.Sprintf("event_entry %s ON e.event_id=%[1]s.event_id", joinAlias))
				clauses = append(clauses, fmt.Sprintf("%s.indexed=1 AND %[1]s.key=?", joinAlias))
				values = append(values, key)
				subclauses := make([]string, 0, len(vals))
				for _, val := range vals {
					subclauses = append(subclauses, fmt.Sprintf("(%s.value=? AND %[1]s.codec=?)", joinAlias))
					values = append(values, val.Value, val.Codec)
				}
				clauses = append(clauses, "("+strings.Join(subclauses, " OR ")+")")
			}
		}
	}

	s := `SELECT
				e.event_id,
				t.height,
				tm.tipset_key_cid,
				e.emitter_addr,
				e.event_index,
				tm.message_cid,
				e.reverted,
				ee.flags,
				ee.key,
				ee.codec,
				ee.value
			FROM tipsets t
			JOIN tipset_messages tm ON t.tipset_key_cid=tm.tipset_key_cid
			JOIN events e ON tm.message_id=e.message_id
			JOIN event_entry ee ON e.event_id=ee.event_id`

	if len(joins) > 0 {
		s = s + ", " + strings.Join(joins, ", ")
	}

	if len(clauses) > 0 {
		s = s + " WHERE " + strings.Join(clauses, " AND ")
	}

	// retain insertion order of event_entry rows with the implicit _rowid_ column
	s += " ORDER BY t.height DESC, ee._rowid_ ASC"
	return values, s
}

func toEthTxHashParam(msg types.ChainMsg) (interface{}, error) {
	smsg, ok := msg.(*types.SignedMessage)
	if !ok || smsg.Signature.Type != crypto.SigTypeDelegated {
		return nil, nil
	}
	hash, err := ethTxHashFromSignedMessage(smsg)
	if err != nil {
		return nil, err
	}
	return hash.String(), nil
}

func ethTxHashFromSignedMessage(smsg *types.SignedMessage) (ethtypes.EthHash, error) {
	if smsg.Signature.Type == crypto.SigTypeDelegated {
		tx, err := ethtypes.EthTransactionFromSignedFilecoinMessage(smsg)
		if err != nil {
			return ethtypes.EthHash{}, xerrors.Errorf("failed to convert from signed message: %w", err)
		}

		return tx.TxHash()
	} else if smsg.Signature.Type == crypto.SigTypeSecp256k1 {
		return ethtypes.EthHashFromCid(smsg.Cid())
	}
	// else BLS message
	return ethtypes.EthHashFromCid(smsg.Message.Cid())
}
