package splitstore

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
)

func TestColdSet(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "coldset")

	makeCid := func(i int) cid.Cid {
		h, err := multihash.Sum([]byte(fmt.Sprintf("cid.%d", i)), multihash.SHA2_256, -1)
		if err != nil {
			t.Fatal(err)
		}

		return cid.NewCidV1(cid.Raw, h)
	}

	const count = 1000
	cids := make([]cid.Cid, 0, count)
	for i := 0; i < count; i++ {
		cids = append(cids, makeCid(i))
	}

	cw, err := NewColdSetWriter(path)
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range cids {
		if err := cw.Write(c); err != nil {
			t.Fatal(err)
		}
	}

	if err := cw.Close(); err != nil {
		t.Fatal(err)
	}

	cr, err := NewColdSetReader(path)
	if err != nil {
		t.Fatal(err)
	}

	index := 0
	err = cr.ForEach(func(c cid.Cid) error {
		if index >= count {
			t.Fatal("too many cids")
		}

		if !c.Equals(cids[index]) {
			t.Fatalf("wrong cid %d; expected %s but got %s", index, cids[index], c)
		}

		index++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := cr.Reset(); err != nil {
		t.Fatal(err)
	}

	index = 0
	err = cr.ForEach(func(c cid.Cid) error {
		if index >= count {
			t.Fatal("too many cids")
		}

		if !c.Equals(cids[index]) {
			t.Fatalf("wrong cid; expected %s but got %s", cids[index], c)
		}

		index++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

}
