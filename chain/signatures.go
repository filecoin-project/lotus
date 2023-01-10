package chain

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/lib/sigs"
	"golang.org/x/xerrors"
)

// AuthenticateMessage authenticates the message by verifying that the supplied
// SignedMessage was signed by the indicated Address, computing the correct
// signature payload depending on the signature type. The supplied Address type
// must be recognized by the registered verifier for the signature type.
func AuthenticateMessage(msg *types.SignedMessage, signer address.Address) error {
	var digest []byte

	switch typ := msg.Signature.Type; typ {
	case crypto.SigTypeDelegated:
		txArgs, err := ethtypes.NewEthTxArgsFromMessage(&msg.Message)
		if err != nil {
			return xerrors.Errorf("failed to reconstruct eth transaction: %w", err)
		}
		msg, err := txArgs.ToRlpUnsignedMsg()
		if err != nil {
			return xerrors.Errorf("failed to repack eth rlp message: %w", err)
		}
		digest = msg
	default:
		digest = msg.Message.Cid().Bytes()
	}

	if err := sigs.Verify(&msg.Signature, signer, digest); err != nil {
		return xerrors.Errorf("secpk message %s has invalid signature: %w", msg.Cid(), err)
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
