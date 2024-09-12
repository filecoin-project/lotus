package types

import (
	"bytes"
	"math/big"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/proof"
)

type Ticket struct {
	VRFProof []byte
}

func (t *Ticket) Quality() float64 {
	ticketHash := blake2b.Sum256(t.VRFProof)
	ticketNum := BigFromBytes(ticketHash[:]).Int
	ticketDenu := big.NewInt(1)
	ticketDenu.Lsh(ticketDenu, 256)
	tv, _ := new(big.Rat).SetFrac(ticketNum, ticketDenu).Float64()
	tq := 1 - tv
	return tq
}

type BeaconEntry struct {
	Round uint64
	Data  []byte
}

func NewBeaconEntry(round uint64, data []byte) BeaconEntry {
	return BeaconEntry{
		Round: round,
		Data:  data,
	}
}

type BlockHeader struct {
	Miner address.Address // 0 unique per block/miner

	Ticket                *Ticket           // 1 unique per block/miner: should be a valid VRF
	ElectionProof         *ElectionProof    // 2 unique per block/miner: should be a valid VRF
	BeaconEntries         []BeaconEntry     // 3 identical for all blocks in same tipset
	WinPoStProof          []proof.PoStProof // 4 unique per block/miner
	Parents               []cid.Cid         // 5 identical for all blocks in same tipset
	ParentWeight          BigInt            // 6 identical for all blocks in same tipset
	Height                abi.ChainEpoch    // 7 identical for all blocks in same tipset
	ParentStateRoot       cid.Cid           // 8 identical for all blocks in same tipset
	ParentMessageReceipts cid.Cid           // 9 identical for all blocks in same tipset
	Messages              cid.Cid           // 10 unique per block
	BLSAggregate          *crypto.Signature // 11 unique per block: aggrregate of BLS messages from above
	Timestamp             uint64            // 12 identical for all blocks in same tipset / hard-tied to the value of Height above
	BlockSig              *crypto.Signature // 13 unique per block/miner: miner signature
	ForkSignaling         uint64            // 14 currently unused/undefined
	ParentBaseFee         abi.TokenAmount   // 15 identical for all blocks in same tipset: the base fee after executing parent tipset

	validated bool // internal, true if the signature has been validated
}

func (blk *BlockHeader) ToStorageBlock() (block.Block, error) {
	data, err := blk.Serialize()
	if err != nil {
		return nil, err
	}

	c, err := abi.CidBuilder.Sum(data)
	if err != nil {
		return nil, err
	}

	return block.NewBlockWithCid(data, c)
}

func (blk *BlockHeader) Cid() cid.Cid {
	sb, err := blk.ToStorageBlock()
	if err != nil {
		panic(err) // Not sure i'm entirely comfortable with this one, needs to be checked
	}

	return sb.Cid()
}

func DecodeBlock(b []byte) (*BlockHeader, error) {
	var blk BlockHeader
	if err := blk.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, err
	}

	return &blk, nil
}

func (blk *BlockHeader) Serialize() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := blk.MarshalCBOR(buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (blk *BlockHeader) LastTicket() *Ticket {
	return blk.Ticket
}

func (blk *BlockHeader) SigningBytes() ([]byte, error) {
	blkcopy := *blk
	blkcopy.BlockSig = nil

	return blkcopy.Serialize()
}

func (blk *BlockHeader) SetValidated() {
	blk.validated = true
}

func (blk *BlockHeader) IsValidated() bool {
	return blk.validated
}

type MsgMeta struct {
	BlsMessages   cid.Cid
	SecpkMessages cid.Cid
}

func (mm *MsgMeta) Cid() cid.Cid {
	b, err := mm.ToStorageBlock()
	if err != nil {
		panic(err) // also maybe sketchy
	}
	return b.Cid()
}

func (mm *MsgMeta) ToStorageBlock() (block.Block, error) {
	var buf bytes.Buffer
	if err := mm.MarshalCBOR(&buf); err != nil {
		return nil, xerrors.Errorf("failed to marshal MsgMeta: %w", err)
	}

	c, err := abi.CidBuilder.Sum(buf.Bytes())
	if err != nil {
		return nil, err
	}

	return block.NewBlockWithCid(buf.Bytes(), c)
}

func CidArrsEqual(a, b []cid.Cid) bool {
	if len(a) != len(b) {
		return false
	}

	// order ignoring compare...
	s := make(map[cid.Cid]bool)
	for _, c := range a {
		s[c] = true
	}

	for _, c := range b {
		if !s[c] {
			return false
		}
	}
	return true
}

func CidArrsSubset(a, b []cid.Cid) bool {
	// order ignoring compare...
	s := make(map[cid.Cid]bool)
	for _, c := range b {
		s[c] = true
	}

	for _, c := range a {
		if !s[c] {
			return false
		}
	}
	return true
}

func CidArrsContains(a []cid.Cid, b cid.Cid) bool {
	for _, elem := range a {
		if elem.Equals(b) {
			return true
		}
	}
	return false
}

func (t *Ticket) Equals(ot *Ticket) bool {
	return bytes.Equal(t.VRFProof, ot.VRFProof)
}
