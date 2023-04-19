package consensus

import (
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/lib/sigs"
)

// AuthenticateMessage authenticates the message by verifying that the supplied
// SignedMessage was signed by the indicated Address, computing the correct
// signature payload depending on the signature type. The supplied Address type
// must be recognized by the registered verifier for the signature type.
func AuthenticateMessage(msg *types.SignedMessage, signer address.Address) error {
	var digest []byte

	typ := msg.Signature.Type
	switch typ {
	case crypto.SigTypeDelegated:
		txArgs, err := ethtypes.EthTxArgsFromUnsignedEthMessage(&msg.Message)
		if err != nil {
			return xerrors.Errorf("failed to reconstruct eth transaction: %w", err)
		}
		roundTripMsg, err := txArgs.ToUnsignedMessage(msg.Message.From)
		if err != nil {
			return xerrors.Errorf("failed to reconstruct filecoin msg: %w", err)
		}

		if !msg.Message.Equals(roundTripMsg) {
			return xerrors.New("ethereum tx failed to roundtrip")
		}

		rlpEncodedMsg, err := txArgs.ToRlpUnsignedMsg()
		if err != nil {
			return xerrors.Errorf("failed to repack eth rlp message: %w", err)
		}
		digest = rlpEncodedMsg
	default:
		digest = msg.Message.Cid().Bytes()
	}

	if err := sigs.Verify(&msg.Signature, signer, digest); err != nil {
		return xerrors.Errorf("message %s has invalid signature (type %d): %w", msg.Cid(), typ, err)
	}
	return nil
}

// IsValidSecpkSigType checks that a signature type is valid for the network
// version, for a "secpk" message.
func IsValidSecpkSigType(nv network.Version, typ crypto.SigType) bool {
	switch {
	case nv < network.Version18:
		return typ == crypto.SigTypeSecp256k1
	default:
		return typ == crypto.SigTypeSecp256k1 || typ == crypto.SigTypeDelegated
	}
}
