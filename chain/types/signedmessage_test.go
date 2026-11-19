package types

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
)

func TestDecodeMessagesRejectTrailingData(t *testing.T) {
	to, err := address.NewIDAddress(1001)
	require.NoError(t, err)
	from, err := address.NewIDAddress(1002)
	require.NoError(t, err)

	msg := Message{
		To:         to,
		From:       from,
		Nonce:      1,
		Value:      big.NewInt(1),
		GasLimit:   1000,
		GasFeeCap:  big.NewInt(100),
		GasPremium: big.NewInt(10),
	}
	smsg := SignedMessage{
		Message:   msg,
		Signature: crypto.Signature{Type: crypto.SigTypeSecp256k1, Data: bytes.Repeat([]byte{0x01}, 65)},
	}

	msgBytes, err := msg.Serialize()
	require.NoError(t, err)
	smsgBytes, err := smsg.Serialize()
	require.NoError(t, err)

	gotMsg, err := DecodeMessage(msgBytes)
	require.NoError(t, err)
	require.Equal(t, msg.Cid(), gotMsg.Cid())

	gotSmsg, err := DecodeSignedMessage(smsgBytes)
	require.NoError(t, err)
	require.Equal(t, smsg.Cid(), gotSmsg.Cid())

	junk := bytes.Repeat([]byte{0xaa}, 128<<10)

	_, err = DecodeMessage(append(msgBytes, junk...))
	require.ErrorContains(t, err, "extraneous data")

	_, err = DecodeSignedMessage(append(smsgBytes, junk...))
	require.ErrorContains(t, err, "extraneous data")
}
