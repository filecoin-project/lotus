package full

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/types"
)

func TestSanityCheckOutgoingMessage(t *testing.T) {
	// fails for invalid delegated address
	badTo, err := address.NewFromString("f410f74aaaaaaaaaaaaaaaaaaaaaaaaac5sh2bf3lgta")
	require.NoError(t, err)
	msg := &types.Message{
		To: badTo,
	}

	err = sanityCheckOutgoingMessage(msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "is a delegated address but not a valid Eth Address")

	// works for valid delegated address
	goodTo, err := address.NewFromString("f410faxfebiima2gp4lduo2k3vt2iuqapuk3logeftky")
	require.NoError(t, err)
	msg = &types.Message{
		To: goodTo,
	}
	err = sanityCheckOutgoingMessage(msg)
	require.NoError(t, err)

	// works for valid non-delegated address
	goodTo, err = address.NewFromString("f1z762skeib2v6zlkvhywmjxbv3dxoiv4hmb6gs4y")
	require.NoError(t, err)
	msg = &types.Message{
		To: goodTo,
	}
	err = sanityCheckOutgoingMessage(msg)
	require.NoError(t, err)
}

func TestMpoolPushInvalidDelegatedAddressFails(t *testing.T) {
	badTo, err := address.NewFromString("f410f74aaaaaaaaaaaaaaaaaaaaaaaaac5sh2bf3lgta")
	require.NoError(t, err)
	module := &MpoolModule{}
	m := &MpoolAPI{
		MpoolModuleAPI: module,
	}
	smsg := &types.SignedMessage{
		Message: types.Message{
			From: badTo,
			To:   badTo,
		},
		Signature: crypto.Signature{
			Type: crypto.SigTypeSecp256k1,
			Data: []byte("signature"),
		},
	}
	_, err = m.MpoolPush(context.Background(), smsg)
	require.Error(t, err)

	require.Contains(t, err.Error(), "is a delegated address but not a valid Eth Address")
}
