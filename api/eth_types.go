package api

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	xerrors "golang.org/x/xerrors"
)

type EthInt int64

func (e EthInt) MarshalJSON() ([]byte, error) {
	return json.Marshal(fmt.Sprintf("0x%x", e))
}

type EthBlock struct {
}

type EthTx struct {
}

type EthTxReceipt struct {
}

type EthAddress [20]byte

type EthHash [32]byte

func (h EthHash) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.String())
}

func fromHexString(s string) (EthHash, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return EthHash{}, xerrors.Errorf("cannot parse cid hash: %w", err)
	}

	if len(b) > 32 {
		return EthHash{}, xerrors.Errorf("length of decoded bytes is longer than 32")
	}

	var h EthHash
	copy(h[32-len(b):], b)

	return h, nil
}

func EthHashFromCid(c cid.Cid) (EthHash, error) {
	return fromHexString(c.Hash().HexString()[8:])
}

func EthHashFromHex(s string) (EthHash, error) {
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = s[2:]
	}
	if len(s)%2 == 1 {
		s = "0" + s
	}
	return fromHexString(s)
}

func (h EthHash) String() string {
	return "0x" + hex.EncodeToString(h[:])
}

func (h EthHash) ToCid() cid.Cid {
	// err is always nil
	mh, _ := multihash.EncodeName(h[:], "blake2b-256")

	return cid.NewCidV1(cid.DagCBOR, mh)
}
