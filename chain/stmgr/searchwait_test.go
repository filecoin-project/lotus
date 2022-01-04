package stmgr_test

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/chain/gen"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
)

func TestSearchForMessageReplacements(t *testing.T) {
	ctx := context.Background()
	cg, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	mts1, err := cg.NextTipSet()
	if err != nil {
		t.Fatal(err)
	}

	m := mts1.Messages[0]

	mts2, err := cg.NextTipSet()
	if err != nil {
		t.Fatal(err)
	}

	// Step 1: Searching for the executed msg with replacements allowed succeeds
	ts, r, mcid, err := cg.StateManager().SearchForMessage(ctx, mts2.TipSet.TipSet(), m.Cid(), 100, true)
	if err != nil {
		t.Fatal(err)
	}

	if !ts.Equals(mts2.TipSet.TipSet()) {
		t.Fatal("searched tipset wasn't as expected")
	}

	if r.ExitCode != 0 {
		t.Fatal("searched msg wasn't successfully executed")
	}

	if mcid != m.Cid() {
		t.Fatal("searched msg wasn't identical to queried msg as expected")
	}

	// Step 2: Searching for the executed msg with replacements disallowed also succeeds
	ts, r, mcid, err = cg.StateManager().SearchForMessage(ctx, mts2.TipSet.TipSet(), m.Cid(), 100, true)
	if err != nil {
		t.Fatal(err)
	}

	if !ts.Equals(mts2.TipSet.TipSet()) {
		t.Fatal("searched tipset wasn't as expected")
	}

	if r.ExitCode != 0 {
		t.Fatal("searched msg wasn't successfully executed")
	}

	if mcid != m.Cid() {
		t.Fatal("searched msg wasn't identical to queried msg as expected")
	}

	// rm is a valid replacement message for m
	rm := m.Message
	rm.GasLimit = m.Message.GasLimit + 1

	rmb, err := rm.ToStorageBlock()
	if err != nil {
		t.Fatal(err)
	}

	err = cg.Blockstore().Put(ctx, rmb)
	if err != nil {
		t.Fatal(err)
	}

	// Step 3: Searching for the replacement msg with replacements allowed succeeds
	ts, r, mcid, err = cg.StateManager().SearchForMessage(ctx, mts2.TipSet.TipSet(), rm.Cid(), 100, true)
	if err != nil {
		t.Fatal(err)
	}

	if !ts.Equals(mts2.TipSet.TipSet()) {
		t.Fatal("searched tipset wasn't as expected")
	}

	if r.ExitCode != 0 {
		t.Fatal("searched msg wasn't successfully executed")
	}

	if mcid == rm.Cid() {
		t.Fatal("searched msg was identical to queried msg, not as expected")
	}

	if mcid != m.Cid() {
		t.Fatal("searched msg wasn't identical to executed msg as expected")
	}

	// Step 4: Searching for the replacement msg with replacements disallowed fails
	_, _, _, err = cg.StateManager().SearchForMessage(ctx, mts2.TipSet.TipSet(), rm.Cid(), 100, false)
	if err == nil {
		t.Fatal("expected search to fail")
	}

	// nrm is NOT a valid replacement message for m
	nrm := m.Message
	nrm.Value = big.Add(m.Message.Value, m.Message.Value)

	nrmb, err := nrm.ToStorageBlock()
	if err != nil {
		t.Fatal(err)
	}

	err = cg.Blockstore().Put(ctx, nrmb)
	if err != nil {
		t.Fatal(err)
	}

	// Step 5: Searching for the not-replacement msg with replacements allowed fails
	_, _, _, err = cg.StateManager().SearchForMessage(ctx, mts2.TipSet.TipSet(), nrm.Cid(), 100, true)
	if err == nil {
		t.Fatal("expected search to fail")
	}

	// Step 6: Searching for the not-replacement msg with replacements disallowed also fails
	_, _, _, err = cg.StateManager().SearchForMessage(ctx, mts2.TipSet.TipSet(), nrm.Cid(), 100, false)
	if err == nil {
		t.Fatal("expected search to fail")
	}

}
