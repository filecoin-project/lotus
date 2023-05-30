package types

import (
	"bytes"
	"fmt"

	"github.com/ipfs/go-cid"
)

type BlockMsg struct {
	Header        *BlockHeader
	BlsMessages   []cid.Cid
	SecpkMessages []cid.Cid
}

func DecodeBlockMsg(b []byte) (*BlockMsg, error) {
	var bm BlockMsg
	data := bytes.NewReader(b)
	if err := bm.UnmarshalCBOR(data); err != nil {
		return nil, err
	}
	if l := data.Len(); l != 0 {
		return nil, fmt.Errorf("extraneous data in BlockMsg CBOR encoding: got %d unexpected bytes", l)
	}
	return &bm, nil
}

func (bm *BlockMsg) Cid() cid.Cid {
	return bm.Header.Cid()
}

func (bm *BlockMsg) Serialize() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := bm.MarshalCBOR(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
