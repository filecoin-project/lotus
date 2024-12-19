package splitstore

import (
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
)

func TestMapMarkSet(t *testing.T) {
	testMarkSet(t, "map")
	testMarkSetRecovery(t, "map")
	testMarkSetMarkMany(t, "map")
	testMarkSetVisitor(t, "map")
	testMarkSetVisitorRecovery(t, "map")
}

func TestBadgerMarkSet(t *testing.T) {
	bs := badgerMarkSetBatchSize
	badgerMarkSetBatchSize = 1
	t.Cleanup(func() {
		badgerMarkSetBatchSize = bs
	})
	testMarkSet(t, "badger")
	testMarkSetRecovery(t, "badger")
	testMarkSetMarkMany(t, "badger")
	testMarkSetVisitor(t, "badger")
	testMarkSetVisitorRecovery(t, "badger")
}

func testMarkSet(t *testing.T, lsType string) {
	path := t.TempDir()

	env, err := OpenMarkSetEnv(path, lsType)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close() //nolint:errcheck

	hotSet, err := env.New("hot", 0)
	if err != nil {
		t.Fatal(err)
	}

	coldSet, err := env.New("cold", 0)
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
		t.Helper()
		has, err := s.Has(cid)
		if err != nil {
			t.Fatal(err)
		}

		if !has {
			t.Fatal("mark not found")
		}
	}

	mustNotHave := func(s MarkSet, cid cid.Cid) {
		t.Helper()
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

	hotSet, err = env.New("hot", 0)
	if err != nil {
		t.Fatal(err)
	}

	coldSet, err = env.New("cold", 0)
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
	path := t.TempDir()

	env, err := OpenMarkSetEnv(path, lsType)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close() //nolint:errcheck

	visitor, err := env.New("test", 0)
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

func testMarkSetVisitorRecovery(t *testing.T, lsType string) {
	path := t.TempDir()

	env, err := OpenMarkSetEnv(path, lsType)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close() //nolint:errcheck

	visitor, err := env.New("test", 0)
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

	if err := visitor.BeginCriticalSection(); err != nil {
		t.Fatal(err)
	}

	mustVisit(visitor, k3)
	mustVisit(visitor, k4)

	mustNotVisit(visitor, k1)
	mustNotVisit(visitor, k2)
	mustNotVisit(visitor, k3)
	mustNotVisit(visitor, k4)

	if err := visitor.Close(); err != nil {
		t.Fatal(err)
	}

	visitor, err = env.Recover("test")
	if err != nil {
		t.Fatal(err)
	}

	mustNotVisit(visitor, k1)
	mustNotVisit(visitor, k2)
	mustNotVisit(visitor, k3)
	mustNotVisit(visitor, k4)

	visitor.EndCriticalSection()

	if err := visitor.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = env.Recover("test")
	if err == nil {
		t.Fatal("expected recovery to fail")
	}
}

func testMarkSetRecovery(t *testing.T, lsType string) {
	path := t.TempDir()

	env, err := OpenMarkSetEnv(path, lsType)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close() //nolint:errcheck

	markSet, err := env.New("test", 0)
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
		t.Helper()
		has, err := s.Has(cid)
		if err != nil {
			t.Fatal(err)
		}

		if !has {
			t.Fatal("mark not found")
		}
	}

	mustNotHave := func(s MarkSet, cid cid.Cid) {
		t.Helper()
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

	if err := markSet.Mark(k1); err != nil {
		t.Fatal(err)
	}
	if err := markSet.Mark(k2); err != nil {
		t.Fatal(err)
	}

	mustHave(markSet, k1)
	mustHave(markSet, k2)
	mustNotHave(markSet, k3)
	mustNotHave(markSet, k4)

	if err := markSet.BeginCriticalSection(); err != nil {
		t.Fatal(err)
	}

	if err := markSet.Mark(k3); err != nil {
		t.Fatal(err)
	}
	if err := markSet.Mark(k4); err != nil {
		t.Fatal(err)
	}

	mustHave(markSet, k1)
	mustHave(markSet, k2)
	mustHave(markSet, k3)
	mustHave(markSet, k4)

	if err := markSet.Close(); err != nil {
		t.Fatal(err)
	}

	markSet, err = env.Recover("test")
	if err != nil {
		t.Fatal(err)
	}

	mustHave(markSet, k1)
	mustHave(markSet, k2)
	mustHave(markSet, k3)
	mustHave(markSet, k4)

	markSet.EndCriticalSection()

	if err := markSet.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = env.Recover("test")
	if err == nil {
		t.Fatal("expected recovery to fail")
	}
}

func testMarkSetMarkMany(t *testing.T, lsType string) {
	path := t.TempDir()

	env, err := OpenMarkSetEnv(path, lsType)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close() //nolint:errcheck

	markSet, err := env.New("test", 0)
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
		t.Helper()
		has, err := s.Has(cid)
		if err != nil {
			t.Fatal(err)
		}

		if !has {
			t.Fatal("mark not found")
		}
	}

	mustNotHave := func(s MarkSet, cid cid.Cid) {
		t.Helper()
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

	if err := markSet.MarkMany([]cid.Cid{k1, k2}); err != nil {
		t.Fatal(err)
	}

	mustHave(markSet, k1)
	mustHave(markSet, k2)
	mustNotHave(markSet, k3)
	mustNotHave(markSet, k4)

	if err := markSet.BeginCriticalSection(); err != nil {
		t.Fatal(err)
	}

	if err := markSet.MarkMany([]cid.Cid{k3, k4}); err != nil {
		t.Fatal(err)
	}

	mustHave(markSet, k1)
	mustHave(markSet, k2)
	mustHave(markSet, k3)
	mustHave(markSet, k4)

	if err := markSet.Close(); err != nil {
		t.Fatal(err)
	}

	markSet, err = env.Recover("test")
	if err != nil {
		t.Fatal(err)
	}

	mustHave(markSet, k1)
	mustHave(markSet, k2)
	mustHave(markSet, k3)
	mustHave(markSet, k4)

	markSet.EndCriticalSection()

	if err := markSet.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = env.Recover("test")
	if err == nil {
		t.Fatal("expected recovery to fail")
	}
}
