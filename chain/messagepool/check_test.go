package messagepool

import (
	"context"
	"fmt"
	"testing"

	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
	"github.com/filecoin-project/lotus/chain/wallet"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
)

func init() {
	_ = logging.SetLogLevel("*", "INFO")
}

func getCheckMessageStatus(statusCode api.CheckStatusCode, msgStatuses []api.MessageCheckStatus) (*api.MessageCheckStatus, error) {
	for i := 0; i < len(msgStatuses); i++ {
		iMsgStatuses := msgStatuses[i]
		if iMsgStatuses.CheckStatus.Code == statusCode {
			return &iMsgStatuses, nil
		}
	}
	return nil, fmt.Errorf("Could not find CheckStatusCode %s", statusCode)
}

func TestCheckMessages(t *testing.T) {
	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	ds := datastore.NewMapDatastore()

	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	if err != nil {
		t.Fatal(err)
	}

	sender, err := w.WalletNew(context.Background(), types.KTSecp256k1)
	if err != nil {
		t.Fatal(err)
	}

	tma.setBalance(sender, 1000e15)
	target := mock.Address(1001)

	var protos []*api.MessagePrototype
	for i := 0; i < 5; i++ {
		msg := &types.Message{
			To:         target,
			From:       sender,
			Value:      types.NewInt(1),
			Nonce:      uint64(i),
			GasLimit:   50000000,
			GasFeeCap:  types.NewInt(minimumBaseFee.Uint64()),
			GasPremium: types.NewInt(1),
			Params:     make([]byte, 2<<10),
		}
		proto := &api.MessagePrototype{
			Message:    *msg,
			ValidNonce: true,
		}
		protos = append(protos, proto)
	}

	messageStatuses, err := mp.CheckMessages(context.TODO(), protos)
	assert.NoError(t, err)
	for i := 0; i < len(messageStatuses); i++ {
		iMsgStatuses := messageStatuses[i]
		for j := 0; j < len(iMsgStatuses); j++ {
			jStatus := iMsgStatuses[i]
			assert.True(t, jStatus.OK)
		}
	}
}

func TestCheckPendingMessages(t *testing.T) {
	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	ds := datastore.NewMapDatastore()

	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	if err != nil {
		t.Fatal(err)
	}

	sender, err := w.WalletNew(context.Background(), types.KTSecp256k1)
	if err != nil {
		t.Fatal(err)
	}

	tma.setBalance(sender, 1000e15)
	target := mock.Address(1001)

	// add a valid message to the pool
	msg := &types.Message{
		To:         target,
		From:       sender,
		Value:      types.NewInt(1),
		Nonce:      0,
		GasLimit:   50000000,
		GasFeeCap:  types.NewInt(minimumBaseFee.Uint64()),
		GasPremium: types.NewInt(1),
		Params:     make([]byte, 2<<10),
	}

	sig, err := w.WalletSign(context.TODO(), sender, msg.Cid().Bytes(), api.MsgMeta{})
	if err != nil {
		panic(err)
	}
	sm := &types.SignedMessage{
		Message:   *msg,
		Signature: *sig,
	}
	mustAdd(t, mp, sm)

	messageStatuses, err := mp.CheckPendingMessages(context.TODO(), sender)
	assert.NoError(t, err)
	for i := 0; i < len(messageStatuses); i++ {
		iMsgStatuses := messageStatuses[i]
		for j := 0; j < len(iMsgStatuses); j++ {
			jStatus := iMsgStatuses[i]
			assert.True(t, jStatus.OK)
		}
	}
}

func TestCheckReplaceMessages(t *testing.T) {
	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	ds := datastore.NewMapDatastore()

	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	if err != nil {
		t.Fatal(err)
	}

	sender, err := w.WalletNew(context.Background(), types.KTSecp256k1)
	if err != nil {
		t.Fatal(err)
	}

	tma.setBalance(sender, 1000e15)
	target := mock.Address(1001)

	// add a valid message to the pool
	msg := &types.Message{
		To:         target,
		From:       sender,
		Value:      types.NewInt(1),
		Nonce:      0,
		GasLimit:   50000000,
		GasFeeCap:  types.NewInt(minimumBaseFee.Uint64()),
		GasPremium: types.NewInt(1),
		Params:     make([]byte, 2<<10),
	}

	sig, err := w.WalletSign(context.TODO(), sender, msg.Cid().Bytes(), api.MsgMeta{})
	if err != nil {
		panic(err)
	}
	sm := &types.SignedMessage{
		Message:   *msg,
		Signature: *sig,
	}
	mustAdd(t, mp, sm)

	// create a new message with the same data, except that it is too big
	var msgs []*types.Message
	invalidmsg := &types.Message{
		To:         target,
		From:       sender,
		Value:      types.NewInt(1),
		Nonce:      0,
		GasLimit:   50000000,
		GasFeeCap:  types.NewInt(minimumBaseFee.Uint64()),
		GasPremium: types.NewInt(1),
		Params:     make([]byte, 128<<10),
	}
	msgs = append(msgs, invalidmsg)

	{
		messageStatuses, err := mp.CheckReplaceMessages(context.TODO(), msgs)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < len(messageStatuses); i++ {
			iMsgStatuses := messageStatuses[i]

			status, err := getCheckMessageStatus(api.CheckStatusMessageSize, iMsgStatuses)
			if err != nil {
				t.Fatal(err)
			}
			// the replacement message should cause a status error
			assert.False(t, status.OK)
		}
	}

}
