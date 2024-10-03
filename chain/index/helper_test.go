package index

import (
	pseudo "math/rand"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/test-go/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

func randomIDAddr(tb testing.TB, rng *pseudo.Rand) address.Address {
	tb.Helper()
	addr, err := address.NewIDAddress(uint64(rng.Int63()))
	require.NoError(tb, err)
	return addr
}

func randomCid(tb testing.TB, rng *pseudo.Rand) cid.Cid {
	tb.Helper()
	cb := cid.V1Builder{Codec: cid.Raw, MhType: mh.IDENTITY}
	c, err := cb.Sum(randomBytes(10, rng))
	require.NoError(tb, err)
	return c
}

func randomBytes(n int, rng *pseudo.Rand) []byte {
	buf := make([]byte, n)
	rng.Read(buf)
	return buf
}

func randomTipsetWithTimestamp(tb testing.TB, rng *pseudo.Rand, h abi.ChainEpoch, parents []cid.Cid, timeStamp uint64) *types.TipSet {
	tb.Helper()

	if timeStamp == 0 {
		timeStamp = uint64(time.Now().Add(time.Duration(h) * builtin.EpochDurationSeconds * time.Second).Unix())
	}

	ts, err := types.NewTipSet([]*types.BlockHeader{
		{
			Height: h,
			Miner:  randomIDAddr(tb, rng),

			Parents: parents,

			Ticket: &types.Ticket{VRFProof: []byte{byte(h % 2)}},

			ParentStateRoot:       randomCid(tb, rng),
			Messages:              randomCid(tb, rng),
			ParentMessageReceipts: randomCid(tb, rng),

			BlockSig:     &crypto.Signature{Type: crypto.SigTypeBLS},
			BLSAggregate: &crypto.Signature{Type: crypto.SigTypeBLS},

			Timestamp: timeStamp,
		},
		{
			Height: h,
			Miner:  randomIDAddr(tb, rng),

			Parents: parents,

			Ticket: &types.Ticket{VRFProof: []byte{byte((h + 1) % 2)}},

			ParentStateRoot:       randomCid(tb, rng),
			Messages:              randomCid(tb, rng),
			ParentMessageReceipts: randomCid(tb, rng),

			BlockSig:     &crypto.Signature{Type: crypto.SigTypeBLS},
			BLSAggregate: &crypto.Signature{Type: crypto.SigTypeBLS},

			Timestamp: timeStamp,
		},
	})

	require.NoError(tb, err)

	return ts
}

func fakeTipSet(tb testing.TB, rng *pseudo.Rand, h abi.ChainEpoch, parents []cid.Cid) *types.TipSet {
	tb.Helper()

	ts, err := types.NewTipSet([]*types.BlockHeader{
		{
			Height: h,
			Miner:  randomIDAddr(tb, rng),

			Parents: parents,

			Ticket: &types.Ticket{VRFProof: []byte{byte(h % 2)}},

			ParentStateRoot:       randomCid(tb, rng),
			Messages:              randomCid(tb, rng),
			ParentMessageReceipts: randomCid(tb, rng),

			BlockSig:     &crypto.Signature{Type: crypto.SigTypeBLS},
			BLSAggregate: &crypto.Signature{Type: crypto.SigTypeBLS},
		},
		{
			Height: h,
			Miner:  randomIDAddr(tb, rng),

			Parents: parents,

			Ticket: &types.Ticket{VRFProof: []byte{byte((h + 1) % 2)}},

			ParentStateRoot:       randomCid(tb, rng),
			Messages:              randomCid(tb, rng),
			ParentMessageReceipts: randomCid(tb, rng),

			BlockSig:     &crypto.Signature{Type: crypto.SigTypeBLS},
			BLSAggregate: &crypto.Signature{Type: crypto.SigTypeBLS},
		},
	})

	require.NoError(tb, err)

	return ts
}

func setupWithHeadIndexed(t *testing.T, headHeight abi.ChainEpoch, rng *pseudo.Rand) (*SqliteIndexer, *types.TipSet, *dummyChainStore) {
	head := fakeTipSet(t, rng, headHeight, []cid.Cid{})
	d := newDummyChainStore()
	d.SetHeaviestTipSet(head)

	s, err := NewSqliteIndexer(":memory:", d, 0, false, 0)
	require.NoError(t, err)
	insertHead(t, s, head, headHeight)

	return s, head, d
}

func cleanup(t *testing.T, s *SqliteIndexer) {
	err := s.Close()
	require.NoError(t, err)
}

func insertHead(t *testing.T, s *SqliteIndexer, head *types.TipSet, height abi.ChainEpoch) {
	headKeyBytes, err := toTipsetKeyCidBytes(head)
	require.NoError(t, err)

	insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: headKeyBytes,
		height:       uint64(height),
		reverted:     false,
		messageCid:   nil,
		messageIndex: -1,
	})
}

func insertEthTxHash(t *testing.T, s *SqliteIndexer, ethTxHash ethtypes.EthHash, messageCid cid.Cid) {
	msgCidBytes := messageCid.Bytes()

	res, err := s.stmts.insertEthTxHashStmt.Exec(ethTxHash.String(), msgCidBytes)
	require.NoError(t, err)
	rowsAffected, err := res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rowsAffected)
}

func insertRandomTipsetAtHeight(t *testing.T, si *SqliteIndexer, height uint64, reverted bool, genesisTime time.Time) (cid.Cid, cid.Cid, int64) {
	rng := pseudo.New(pseudo.NewSource(time.Now().UnixNano()))
	tsCid := randomCid(t, rng)
	msgCid := randomCid(t, rng)

	msgID := insertTipsetMessage(t, si, tipsetMessage{
		tipsetKeyCid: tsCid.Bytes(),
		height:       height,
		reverted:     reverted,
		messageCid:   msgCid.Bytes(),
		messageIndex: 0,
	})

	eventID := insertEvent(t, si, event{
		messageID:   msgID,
		eventIndex:  0,
		emitterAddr: randomIDAddr(t, rng).Bytes(),
		reverted:    reverted,
	})

	insertEthTxHashAtTimeStamp(
		t,
		si,
		ethtypes.EthHash(randomBytes(32, rng)),
		msgCid,
		genesisTime.Add(time.Duration(height)*builtin.EpochDurationSeconds*time.Second).Unix(),
	)

	insertEventEntry(t, si, eventEntry{
		eventID: eventID,
		indexed: true,
		flags:   []byte("test_data"),
		key:     "test_key",
		codec:   0,
		value:   []byte("test_value"),
	})

	return tsCid, msgCid, eventID
}

func insertEthTxHashAtTimeStamp(t *testing.T, s *SqliteIndexer, ethTxHash ethtypes.EthHash, messageCid cid.Cid, timestamp int64) {
	msgCidBytes := messageCid.Bytes()

	res, err := s.db.Exec("INSERT INTO eth_tx_hash (tx_hash, message_cid, inserted_at) VALUES (?, ?, ?)", ethTxHash.String(), msgCidBytes, timestamp)
	require.NoError(t, err)
	rowsAffected, err := res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rowsAffected)
}

func insertTipsetMessage(t *testing.T, s *SqliteIndexer, ts tipsetMessage) int64 {
	res, err := s.stmts.insertTipsetMessageStmt.Exec(ts.tipsetKeyCid, ts.height, ts.reverted, ts.messageCid, ts.messageIndex)
	require.NoError(t, err)

	rowsAffected, err := res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rowsAffected)

	messageID, err := res.LastInsertId()
	require.NoError(t, err)
	require.NotEqual(t, int64(0), messageID)

	// read back the message to verify it was inserted correctly
	verifyTipsetMessage(t, s, messageID, ts)

	return messageID
}

func insertEvent(t *testing.T, s *SqliteIndexer, e event) int64 {
	res, err := s.stmts.insertEventStmt.Exec(e.messageID, e.eventIndex, e.emitterAddr, e.reverted)
	require.NoError(t, err)

	rowsAffected, err := res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rowsAffected)

	eventID, err := res.LastInsertId()
	require.NoError(t, err)
	require.NotEqual(t, int64(0), eventID)

	verifyEvent(t, s, eventID, e)

	return eventID
}

func insertEventEntry(t *testing.T, s *SqliteIndexer, ee eventEntry) {
	res, err := s.stmts.insertEventEntryStmt.Exec(ee.eventID, ee.indexed, ee.flags, ee.key, ee.codec, ee.value)
	require.NoError(t, err)

	rowsAffected, err := res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rowsAffected)
}
