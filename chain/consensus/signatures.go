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
	signatureType := msg.Signature.Type
	signatureCopy := msg.Signature

	switch signatureType {
	case crypto.SigTypeDelegated:
		signatureCopy.Data = make([]byte, len(msg.Signature.Data))
		copy(signatureCopy.Data, msg.Signature.Data)
		ethTx, err := ethtypes.EthTransactionFromSignedFilecoinMessage(msg)
		if err != nil {
			return xerrors.Errorf("failed to reconstruct Ethereum transaction: %w", err)
		}

		filecoinMsg, err := ethTx.ToUnsignedFilecoinMessage(msg.Message.From)
		if err != nil {
			return xerrors.Errorf("failed to reconstruct Filecoin message: %w", err)
		}

		if !msg.Message.Equals(filecoinMsg) {
			return xerrors.New("Ethereum transaction roundtrip mismatch")
		}

		rlpEncodedMsg, err := ethTx.ToRlpUnsignedMsg()
		if err != nil {
			return xerrors.Errorf("failed to encode RLP message: %w", err)
		}
		digest = rlpEncodedMsg
		signatureCopy.Data, err = ethTx.ToVerifiableSignature(signatureCopy.Data)
		if err != nil {
			return xerrors.Errorf("failed to verify signature: %w", err)
		}
	default:
		digest = msg.Message.Cid().Bytes()
	}

	if err := sigs.Verify(&signatureCopy, signer, digest); err != nil {
		return xerrors.Errorf("invalid signature for message %s (type %d): %w", msg.Cid(), signatureType, err)
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
