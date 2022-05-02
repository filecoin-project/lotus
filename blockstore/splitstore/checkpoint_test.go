package splitstore

import (
	"path/filepath"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
)

func TestCheckpoint(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "checkpoint")

	makeCid := func(key string) cid.Cid {
		h, err := multihash.Sum([]byte(key), multihash.SHA2_256, -1)
		if err != nil {
			t.Fatal(err)
		}

		return cid.NewCidV1(cid.Raw, h)
	}

	k1 := makeCid("a")
	k2 := makeCid("b")
	k3 := makeCid("c")
	k4 := makeCid("d")

	cp, err := NewCheckpoint(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := cp.Set(k1); err != nil {
		t.Fatal(err)
	}
	if err := cp.Set(k2); err != nil {
		t.Fatal(err)
	}

	if err := cp.Close(); err != nil {
		t.Fatal(err)
	}

	cp, start, err := OpenCheckpoint(path)
	if err != nil {
		t.Fatal(err)
	}
	if !start.Equals(k2) {
		t.Fatalf("expected start to be %s; got %s", k2, start)
	}

	if err := cp.Set(k3); err != nil {
		t.Fatal(err)
	}
	if err := cp.Set(k4); err != nil {
		t.Fatal(err)
	}

	if err := cp.Close(); err != nil {
		t.Fatal(err)
	}

	cp, start, err = OpenCheckpoint(path)
	if err != nil {
		t.Fatal(err)
	}
	if !start.Equals(k4) {
		t.Fatalf("expected start to be %s; got %s", k4, start)
	}

	if err := cp.Close(); err != nil {
		t.Fatal(err)
	}

	// also test correct operation with an empty checkpoint
	cp, err = NewCheckpoint(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := cp.Close(); err != nil {
		t.Fatal(err)
	}

	cp, start, err = OpenCheckpoint(path)
	if err != nil {
		t.Fatal(err)
	}

	if start.Defined() {
		t.Fatal("expected start to be undefined")
	}

	if err := cp.Set(k1); err != nil {
		t.Fatal(err)
	}
	if err := cp.Set(k2); err != nil {
		t.Fatal(err)
	}

	if err := cp.Close(); err != nil {
		t.Fatal(err)
	}

	cp, start, err = OpenCheckpoint(path)
	if err != nil {
		t.Fatal(err)
	}
	if !start.Equals(k2) {
		t.Fatalf("expected start to be %s; got %s", k2, start)
	}

	if err := cp.Set(k3); err != nil {
		t.Fatal(err)
	}
	if err := cp.Set(k4); err != nil {
		t.Fatal(err)
	}

	if err := cp.Close(); err != nil {
		t.Fatal(err)
	}

	cp, start, err = OpenCheckpoint(path)
	if err != nil {
		t.Fatal(err)
	}
	if !start.Equals(k4) {
		t.Fatalf("expected start to be %s; got %s", k4, start)
	}

	if err := cp.Close(); err != nil {
		t.Fatal(err)
	}

}
