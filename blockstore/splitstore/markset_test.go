package splitstore

import (
	"io/ioutil"
	"os"
	"testing"

	cid "github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
)

func TestMapMarkSet(t *testing.T) {
	testMarkSet(t, "map")
	testMarkSetVisitor(t, "map")
}

func TestBadgerMarkSet(t *testing.T) {
	bs := badgerMarkSetBatchSize
	badgerMarkSetBatchSize = 1
	t.Cleanup(func() {
		badgerMarkSetBatchSize = bs
	})
	testMarkSet(t, "badger")
	testMarkSetVisitor(t, "badger")
}

func testMarkSet(t *testing.T, lsType string) {
	t.Helper()

	path, err := ioutil.TempDir("", "markset.*")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = os.RemoveAll(path)
	})

	env, err := OpenMarkSetEnv(path, lsType)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close() //nolint:errcheck

	hotSet, err := env.Create("hot", 0)
	if err != nil {
		t.Fatal(err)
	}

	coldSet, err := env.Create("cold", 0)
	if err != nil {
		t.Fatal(err)
	}

	makeCid := func(key string) cid.Cid {
		h, err := multihash.Sum([]byte(key), multihash.SHA2_256, -1)
		if err != nil {
			t.Fatal(err)
		}

		return cid.NewCidV1(cid.Raw, h)
	}

	mustHave := func(s MarkSet, cid cid.Cid) {
		has, err := s.Has(cid)
		if err != nil {
			t.Fatal(err)
		}

		if !has {
			t.Fatal("mark not found")
		}
	}

	mustNotHave := func(s MarkSet, cid cid.Cid) {
		has, err := s.Has(cid)
		if err != nil {
			t.Fatal(err)
		}

		if has {
			t.Fatal("unexpected mark")
		}
	}

	k1 := makeCid("a")
	k2 := makeCid("b")
	k3 := makeCid("c")
	k4 := makeCid("d")

	hotSet.Mark(k1)  //nolint
	hotSet.Mark(k2)  //nolint
	coldSet.Mark(k3) //nolint

	mustHave(hotSet, k1)
	mustHave(hotSet, k2)
	mustNotHave(hotSet, k3)
	mustNotHave(hotSet, k4)

	mustNotHave(coldSet, k1)
	mustNotHave(coldSet, k2)
	mustHave(coldSet, k3)
	mustNotHave(coldSet, k4)

	// close them and reopen to redo the dance

	err = hotSet.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = coldSet.Close()
	if err != nil {
		t.Fatal(err)
	}

	hotSet, err = env.Create("hot", 0)
	if err != nil {
		t.Fatal(err)
	}

	coldSet, err = env.Create("cold", 0)
	if err != nil {
		t.Fatal(err)
	}

	hotSet.Mark(k3)  //nolint
	hotSet.Mark(k4)  //nolint
	coldSet.Mark(k1) //nolint

	mustNotHave(hotSet, k1)
	mustNotHave(hotSet, k2)
	mustHave(hotSet, k3)
	mustHave(hotSet, k4)

	mustHave(coldSet, k1)
	mustNotHave(coldSet, k2)
	mustNotHave(coldSet, k3)
	mustNotHave(coldSet, k4)

	err = hotSet.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = coldSet.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func testMarkSetVisitor(t *testing.T, lsType string) {
	t.Helper()

	path, err := ioutil.TempDir("", "markset.*")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = os.RemoveAll(path)
	})

	env, err := OpenMarkSetEnv(path, lsType)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close() //nolint:errcheck

	visitor, err := env.CreateVisitor("test", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer visitor.Close() //nolint:errcheck

	makeCid := func(key string) cid.Cid {
		h, err := multihash.Sum([]byte(key), multihash.SHA2_256, -1)
		if err != nil {
			t.Fatal(err)
		}

		return cid.NewCidV1(cid.Raw, h)
	}

	mustVisit := func(v ObjectVisitor, cid cid.Cid) {
		visit, err := v.Visit(cid)
		if err != nil {
			t.Fatal(err)
		}

		if !visit {
			t.Fatal("object should be visited")
		}
	}

	mustNotVisit := func(v ObjectVisitor, cid cid.Cid) {
		visit, err := v.Visit(cid)
		if err != nil {
			t.Fatal(err)
		}

		if visit {
			t.Fatal("unexpected visit")
		}
	}

	k1 := makeCid("a")
	k2 := makeCid("b")
	k3 := makeCid("c")
	k4 := makeCid("d")

	mustVisit(visitor, k1)
	mustVisit(visitor, k2)
	mustVisit(visitor, k3)
	mustVisit(visitor, k4)

	mustNotVisit(visitor, k1)
	mustNotVisit(visitor, k2)
	mustNotVisit(visitor, k3)
	mustNotVisit(visitor, k4)
}
