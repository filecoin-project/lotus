package smm

import (
    "fmt"
    laddress "github.com/filecoin-project/lotus/chain/address"
    "github.com/filecoin-project/lotus/chain/store"
    "github.com/filecoin-project/lotus/chain/types"
    "github.com/ipfs/go-cid"
    "math/rand"
    "testing"
    "time"
)

func testTipset() (ts *types.TipSet, err error) {
    var c cid.Cid

    // Copied from BlockHeader test
    c, err = cid.Decode("bafyreicmaj5hhoy5mgqvamfhgexxyergw7hdeshizghodwkjg6qmpoco7i")

    r := rand.New(rand.NewSource(time.Now().UnixNano()))
    height := uint64(r.Intn(20))

    blockHeaders := make([]*types.BlockHeader, 3)
    for idx, _ := range blockHeaders {
        header := new(types.BlockHeader)
        header.Miner, err = laddress.NewIDAddress(uint64(r.Uint64()))
        if err != nil {
            return
        }
        header.Height = height
        header.ParentStateRoot = c
        header.ParentMessageReceipts = c
        header.Messages = c
        header.Parents = make([]cid.Cid, 0)
        header.Tickets = make([]*types.Ticket, 4)
        for i := 0; i < len(header.Tickets); i++ {
            vrfstring := fmt.Sprintf("Hello world %d", r.Intn(100))
            header.Tickets[i] = &types.Ticket{
                VRFProof: []byte(vrfstring),
            }
        }
        blockHeaders[idx] = header

    }
    ts, err = types.NewTipSet(blockHeaders)
    return
}

func Test_StateID(t *testing.T) {
    t.Helper()
    var err error
    var ts *types.TipSet
    ts, err = testTipset()
    if err != nil {
        t.Fatal(err)
    }
    var stateID *StateKey
    stateID, err = tipset2statekey(ts)
    if err != nil {
        t.Fatal(err)
    }

    var cids []cid.Cid
    cids, err = statekey2cids(stateID)
    if err != nil {
        t.Fatal(err)
    }
    tscids := ts.Cids()
    if len(cids) != len(tscids) {
        t.Fatalf("failed to marshall/unmarshall StateKey")
    }

    for idx, id := range cids {
        if id != tscids[idx] {
            t.Fatalf("failed to marshall/unmarshall StateKey")
        }
    }
}

func Test_AddressCompat(t *testing.T) {
    t.Helper()
    var l1, l2 laddress.Address
    var err error
    l1, err = laddress.NewActorAddress([]byte("Hello World!"))
    if err != nil {
        t.Fatalf("failed to construct Lotus address from ID")
    }

    addr := Address(l1.String())
    l2, err = laddress.NewFromString(string(addr))
    if err != nil {
        t.Fatalf("failed to construct Lotus address from Node address")
    }
    if l1 != l2 {
        t.Fatalf("Lotus and Node adddresses are not compatible")
    }
}

func Test_StateChangesFromHeadChangesTo(t *testing.T) {
    t.Helper()
    var err error
    headChanges := make([]*store.HeadChange, 5)
    for i := 0; i < len(headChanges); i++ {
        var ts *types.TipSet
        ts, err = testTipset()
        if err != nil {
            t.Fatal(err)
        }
        headChanges[i] = &store.HeadChange{
            Type: fmt.Sprintf("Type %d", 1 + i),
            Val: ts,
        }
    }
    var changes []*StateChange
    changes, err = stateChangesFromHeadChanges(headChanges)
    if err != nil {
        t.Fatal(err)
    }
    if len(changes) != len(headChanges) {
        t.Fatalf("error tranforming []*HeadChange to []*StateChange")
    }
    for idx, headChange := range headChanges {
        stateChange := changes[idx]
        if headChange.Type != stateChange.Type {
            t.Fatalf("error tranforming *HeadChange to *StateChange")
        }
        if headChange.Val.Height() != uint64(stateChange.Epoch) {
            t.Fatalf("error tranforming *HeadChange to *StateChange")
        }
        var cids []cid.Cid
        cids, err = statekey2cids(stateChange.StateKey)
        if err != nil {
            t.Fatal(err)
        }
        for idx, cid := range headChange.Val.Cids() {
            if cid != cids[idx] {
                t.Fatalf("error tranforming *HeadChange to *StateChange")
            }
        }

    }
}