package types

import (
	"bytes"
	"encoding/json"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"golang.org/x/xerrors"
)

func (sm *SignedMessage) ToStorageBlock() (block.Block, error) {
	if sm.Signature.Type == crypto.SigTypeBLS {
		return sm.Message.ToStorageBlock()
	}

	data, err := sm.Serialize()
	if err != nil {
		return nil, err
	}

	pref := cid.NewPrefixV1(cid.DagCBOR, multihash.BLAKE2B_MIN+31)
	c, err := pref.Sum(data)
	if err != nil {
		return nil, err
	}

	return block.NewBlockWithCid(data, c)
}

func (sm *SignedMessage) Cid() cid.Cid {
	if sm.Signature.Type == crypto.SigTypeBLS {
		return sm.Message.Cid()
	}

	sb, err := sm.ToStorageBlock()
	if err != nil {
		panic(err)
	}

	return sb.Cid()
}

type SignedMessage struct {
	Message   Message
	Signature crypto.Signature
}

func DecodeSignedMessage(data []byte) (*SignedMessage, error) {
	var msg SignedMessage
	if err := msg.UnmarshalCBOR(bytes.NewReader(data)); err != nil {
		return nil, err
	}

	return &msg, nil
}

func (sm *SignedMessage) Serialize() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := sm.MarshalCBOR(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *SignedMessage) ChainLength() int {
	ser, err := m.Serialize()
	if err != nil {
		panic(err)
	}
	return len(ser)
}

func (sm *SignedMessage) Size() int {
	serdata, err := sm.Serialize()
	if err != nil {
		log.Errorf("serializing message failed: %s", err)
		return 0
	}

	return len(serdata)
}

func (sm *SignedMessage) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return xerrors.Errorf("expected at least 1 byte")
	}

	switch b[0] {
	case '"':
		var b []byte
		if err := json.Unmarshal(b, &b); err != nil {
			return err
		}

		dm, err := DecodeSignedMessage(b)
		if err != nil {
			return err
		}

		*sm = *dm

		return nil
	case '{':
		msg := struct {
			Message   Message
			Signature crypto.Signature
		}{}

		if err := json.Unmarshal(b, &msg); err != nil {
			return err
		}

		sm.Message = msg.Message
		sm.Signature = msg.Signature

		return nil
	default:
		return xerrors.Errorf("unexpected token '%s'", b[0])
	}
}

func (sm *SignedMessage) VMMessage() *Message {
	return &sm.Message
}
