package types

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("types")

type TipSet struct {
	cids   []cid.Cid
	blks   []*BlockHeader
	height uint64
}

// why didnt i just export the fields? Because the struct has methods with the
// same names already
type expTipSet struct {
	Cids   []cid.Cid
	Blocks []*BlockHeader
	Height uint64
}

func (ts *TipSet) MarshalJSON() ([]byte, error) {
	return json.Marshal(expTipSet{
		Cids:   ts.cids,
		Blocks: ts.blks,
		Height: ts.height,
	})
}

func (ts *TipSet) UnmarshalJSON(b []byte) error {
	var ets expTipSet
	if err := json.Unmarshal(b, &ets); err != nil {
		return err
	}

	ts.cids = ets.Cids
	ts.blks = ets.Blocks
	ts.height = ets.Height
	return nil
}

func NewTipSet(blks []*BlockHeader) (*TipSet, error) {
	/*
		sort.Slice(blks, func(i, j int) bool {
			a := blks[i].LastTicket()
			b := blks[j].LastTicket()
			return bytes.Compare(a.VDFResult, b.VDFResult) < 0
		})
	*/

	var ts TipSet
	ts.cids = []cid.Cid{blks[0].Cid()}
	ts.blks = blks
	for _, b := range blks[1:] {
		if b.Height != blks[0].Height {
			return nil, fmt.Errorf("cannot create tipset with mismatching heights")
		}
		ts.cids = append(ts.cids, b.Cid())
	}
	ts.height = blks[0].Height

	return &ts, nil
}

func (ts *TipSet) Cids() []cid.Cid {
	return ts.cids
}

func (ts *TipSet) Height() uint64 {
	return ts.height
}

func (ts *TipSet) Weight() BigInt {
	// TODO: implement correctly
	log.Warn("Called TipSet.Weight: TODO: correct implementation")
	return BigAdd(ts.blks[0].ParentWeight, NewInt(1))
}

func (ts *TipSet) Parents() []cid.Cid {
	return ts.blks[0].Parents
}

func (ts *TipSet) Blocks() []*BlockHeader {
	return ts.blks
}

func (ts *TipSet) Equals(ots *TipSet) bool {
	if len(ts.blks) != len(ots.blks) {
		return false
	}

	for i, b := range ts.blks {
		if b.Cid() != ots.blks[i].Cid() {
			return false
		}
	}

	return true
}

func (t *Ticket) Less(o *Ticket) bool {
	return bytes.Compare(t.VDFResult, o.VDFResult) < 0
}

func (ts *TipSet) MinTicket() *Ticket {
	b := ts.MinTicketBlock()
	return b.Tickets[len(b.Tickets)-1]
}

func (ts *TipSet) MinTimestamp() uint64 {
	minTs := ts.Blocks()[0].Timestamp
	for _, bh := range ts.Blocks()[1:] {
		if bh.Timestamp < minTs {
			minTs = bh.Timestamp
		}
	}
	return minTs
}

func (ts *TipSet) MinTicketBlock() *BlockHeader {
	blks := ts.Blocks()

	min := blks[0]

	for _, b := range blks[1:] {
		if b.LastTicket().Less(min.LastTicket()) {
			min = b
		}
	}

	return min
}
