package drivers

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	vtypes "github.com/filecoin-project/oni/tvx/chain/types"
)

type TipSetMessageBuilder struct {
	driver *TestDriver

	bbs []*BlockBuilder
}

func NewTipSetMessageBuilder(testDriver *TestDriver) *TipSetMessageBuilder {
	return &TipSetMessageBuilder{
		driver: testDriver,
		bbs:    nil,
	}
}

func (t *TipSetMessageBuilder) WithBlockBuilder(bb *BlockBuilder) *TipSetMessageBuilder {
	t.bbs = append(t.bbs, bb)
	return t
}

func (t *TipSetMessageBuilder) Apply() vtypes.ApplyTipSetResult {
	result := t.apply()

	t.Clear()
	return result
}

func (t *TipSetMessageBuilder) ApplyAndValidate() vtypes.ApplyTipSetResult {
	result := t.apply()

	t.validateResult(result)

	t.Clear()
	return result
}

func (tb *TipSetMessageBuilder) apply() vtypes.ApplyTipSetResult {
	var blks []vtypes.BlockMessagesInfo
	for _, b := range tb.bbs {
		blks = append(blks, b.build())
	}
	result, err := tb.driver.applier.ApplyTipSetMessages(tb.driver.ExeCtx.Epoch, blks, tb.driver.Randomness())
	require.NoError(T, err)

	//t.driver.StateTracker.TrackResult(result)
	return result
}

func (tb *TipSetMessageBuilder) validateResult(result vtypes.ApplyTipSetResult) {
	expected := []ExpectedResult{}
	for _, b := range tb.bbs {
		expected = append(expected, b.expectedResults...)
	}

	if len(result.Receipts) > len(expected) {
		T.Fatalf("ApplyTipSetMessages returned more result than expected. Expected: %d, Actual: %d", len(expected), len(result.Receipts))
		return
	}

	for i := range result.Receipts {
		if tb.driver.Config.ValidateExitCode() {
			assert.Equal(T, expected[i].ExitCode, result.Receipts[i].ExitCode, "Message Number: %d Expected ExitCode: %s Actual ExitCode: %s", i, expected[i].ExitCode.Error(), result.Receipts[i].ExitCode.Error())
		}
		if tb.driver.Config.ValidateReturnValue() {
			assert.Equal(T, expected[i].ReturnVal, result.Receipts[i].ReturnValue, "Message Number: %d Expected ReturnValue: %v Actual ReturnValue: %v", i, expected[i].ReturnVal, result.Receipts[i].ReturnValue)
		}
	}
}

func (t *TipSetMessageBuilder) Clear() {
	t.bbs = nil
}

type BlockBuilder struct {
	TD *TestDriver

	miner       address.Address
	ticketCount int64

	secpMsgs []*types.SignedMessage
	blsMsgs  []*types.Message

	expectedResults []ExpectedResult
}

type ExpectedResult struct {
	ExitCode  exitcode.ExitCode
	ReturnVal []byte
}

func NewBlockBuilder(td *TestDriver, miner address.Address) *BlockBuilder {
	return &BlockBuilder{
		TD:              td,
		miner:           miner,
		ticketCount:     1,
		secpMsgs:        nil,
		blsMsgs:         nil,
		expectedResults: nil,
	}
}

func (bb *BlockBuilder) addResult(code exitcode.ExitCode, retval []byte) {
	bb.expectedResults = append(bb.expectedResults, ExpectedResult{
		ExitCode:  code,
		ReturnVal: retval,
	})
}

func (bb *BlockBuilder) WithBLSMessageOk(blsMsg *types.Message) *BlockBuilder {
	bb.blsMsgs = append(bb.blsMsgs, blsMsg)
	bb.addResult(exitcode.Ok, EmptyReturnValue)
	return bb
}

func (bb *BlockBuilder) WithBLSMessageDropped(blsMsg *types.Message) *BlockBuilder {
	bb.blsMsgs = append(bb.blsMsgs, blsMsg)
	return bb
}

func (bb *BlockBuilder) WithBLSMessageAndCode(bm *types.Message, code exitcode.ExitCode) *BlockBuilder {
	bb.blsMsgs = append(bb.blsMsgs, bm)
	bb.addResult(code, EmptyReturnValue)
	return bb
}

func (bb *BlockBuilder) WithBLSMessageAndRet(bm *types.Message, retval []byte) *BlockBuilder {
	bb.blsMsgs = append(bb.blsMsgs, bm)
	bb.addResult(exitcode.Ok, retval)
	return bb
}

func (bb *BlockBuilder) WithSECPMessageAndCode(bm *types.Message, code exitcode.ExitCode) *BlockBuilder {
	secpMsg := bb.toSignedMessage(bm)
	bb.secpMsgs = append(bb.secpMsgs, secpMsg)
	bb.addResult(code, EmptyReturnValue)
	return bb
}

func (bb *BlockBuilder) WithSECPMessageAndRet(bm *types.Message, retval []byte) *BlockBuilder {
	secpMsg := bb.toSignedMessage(bm)
	bb.secpMsgs = append(bb.secpMsgs, secpMsg)
	bb.addResult(exitcode.Ok, retval)
	return bb
}

func (bb *BlockBuilder) WithSECPMessageOk(bm *types.Message) *BlockBuilder {
	secpMsg := bb.toSignedMessage(bm)
	bb.secpMsgs = append(bb.secpMsgs, secpMsg)
	bb.addResult(exitcode.Ok, EmptyReturnValue)
	return bb
}

func (bb *BlockBuilder) WithSECPMessageDropped(bm *types.Message) *BlockBuilder {
	secpMsg := bb.toSignedMessage(bm)
	bb.secpMsgs = append(bb.secpMsgs, secpMsg)
	return bb
}

func (bb *BlockBuilder) WithTicketCount(count int64) *BlockBuilder {
	bb.ticketCount = count
	return bb
}

func (bb *BlockBuilder) toSignedMessage(m *types.Message) *types.SignedMessage {
	from := m.From
	if from.Protocol() == address.ID {
		from = bb.TD.ActorPubKey(from)
	}
	if from.Protocol() != address.SECP256K1 {
		T.Fatalf("Invalid address for SECP signature, address protocol: %v", from.Protocol())
	}
	raw, err := m.Serialize()
	require.NoError(T, err)

	sig, err := bb.TD.Wallet().Sign(from, raw)
	require.NoError(T, err)

	return &types.SignedMessage{
		Message:   *m,
		Signature: sig,
	}
}

func (bb *BlockBuilder) build() vtypes.BlockMessagesInfo {
	return vtypes.BlockMessagesInfo{
		BLSMessages:  bb.blsMsgs,
		SECPMessages: bb.secpMsgs,
		Miner:        bb.miner,
		TicketCount:  bb.ticketCount,
	}
}
