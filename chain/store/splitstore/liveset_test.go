package splitstore

import (
	"os"
	"testing"

	"github.com/bmatsuo/lmdb-go/lmdb"

	cid "github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
)

func TestLiveSet(t *testing.T) {
	env, err := lmdb.NewEnv()
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close() //nolint:errcheck

	if err = env.SetMapSize(1 << 30); err != nil {
		t.Fatal(err)
	}
	if err = env.SetMaxDBs(2); err != nil {
		t.Fatal(err)
	}
	if err = env.SetMaxReaders(1); err != nil {
		t.Fatal(err)
	}

	err = os.MkdirAll("/tmp/liveset-test", 0777)
	if err != nil {
		t.Fatal(err)
	}

	err = env.Open("/tmp/liveset-test", lmdb.NoSync|lmdb.WriteMap|lmdb.MapAsync|lmdb.NoReadahead, 0777)
	if err != nil {
		t.Fatal(err)
	}

	hotSet, err := NewLiveSet(env, "hot")
	if err != nil {
		t.Fatal(err)
	}

	coldSet, err := NewLiveSet(env, "cold")
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

	mustHave := func(s LiveSet, cid cid.Cid) {
		has, err := s.Has(cid)
		if err != nil {
			t.Fatal(err)
		}

		if !has {
			t.Fatal("mark not found")
		}
	}

	mustNotHave := func(s LiveSet, cid cid.Cid) {
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

	hotSet, err = NewLiveSet(env, "hot")
	if err != nil {
		t.Fatal(err)
	}

	coldSet, err = NewLiveSet(env, "cold")
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
