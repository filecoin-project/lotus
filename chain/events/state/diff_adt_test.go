package state

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbornode "github.com/ipfs/go-ipld-cbor"
	typegen "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
)

func TestDiffAdtArray(t *testing.T) {
	ctxstoreA := newContextStore()
	ctxstoreB := newContextStore()

	arrA := adt.MakeEmptyArray(ctxstoreA)
	arrB := adt.MakeEmptyArray(ctxstoreB)

	require.NoError(t, arrA.Set(0, runtime.CBORBytes([]byte{0}))) // delete

	require.NoError(t, arrA.Set(1, runtime.CBORBytes([]byte{0}))) // modify
	require.NoError(t, arrB.Set(1, runtime.CBORBytes([]byte{1})))

	require.NoError(t, arrA.Set(2, runtime.CBORBytes([]byte{1}))) // delete

	require.NoError(t, arrA.Set(3, runtime.CBORBytes([]byte{0}))) // noop
	require.NoError(t, arrB.Set(3, runtime.CBORBytes([]byte{0})))

	require.NoError(t, arrA.Set(4, runtime.CBORBytes([]byte{0}))) // modify
	require.NoError(t, arrB.Set(4, runtime.CBORBytes([]byte{6})))

	require.NoError(t, arrB.Set(5, runtime.CBORBytes{8})) // add
	require.NoError(t, arrB.Set(6, runtime.CBORBytes{9})) // add

	changes := &TestAdtDiff{
		Added:    []runtime.CBORBytes{},
		Modified: []TestAdtDiffModified{},
		Removed:  []runtime.CBORBytes{},
	}

	assert.NoError(t, DiffAdtArray(arrA, arrB, changes))
	assert.NotNil(t, changes)

	assert.Equal(t, 2, len(changes.Added))
	assert.EqualValues(t, []byte{8}, changes.Added[0])
	assert.EqualValues(t, []byte{9}, changes.Added[1])

	assert.Equal(t, 2, len(changes.Modified))
	assert.EqualValues(t, []byte{0}, changes.Modified[0].From)
	assert.EqualValues(t, []byte{1}, changes.Modified[0].To)
	assert.EqualValues(t, []byte{0}, changes.Modified[1].From)
	assert.EqualValues(t, []byte{6}, changes.Modified[1].To)

	assert.Equal(t, 2, len(changes.Removed))
	assert.EqualValues(t, []byte{0}, changes.Removed[0])
	assert.EqualValues(t, []byte{1}, changes.Removed[1])
}

type TestAdtDiff struct {
	Added    []runtime.CBORBytes
	Modified []TestAdtDiffModified
	Removed  []runtime.CBORBytes
}

var _ AdtArrayDiff = &TestAdtDiff{}

type TestAdtDiffModified struct {
	From runtime.CBORBytes
	To   runtime.CBORBytes
}

func (t *TestAdtDiff) Add(val *typegen.Deferred) error {
	v := new(runtime.CBORBytes)
	err := v.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	t.Added = append(t.Added, *v)
	return nil
}

func (t *TestAdtDiff) Modify(from, to *typegen.Deferred) error {
	vFrom := new(runtime.CBORBytes)
	err := vFrom.UnmarshalCBOR(bytes.NewReader(from.Raw))
	if err != nil {
		return err
	}

	vTo := new(runtime.CBORBytes)
	err = vTo.UnmarshalCBOR(bytes.NewReader(to.Raw))
	if err != nil {
		return err
	}

	if !bytes.Equal(*vFrom, *vTo) {
		t.Modified = append(t.Modified, TestAdtDiffModified{
			From: *vFrom,
			To:   *vTo,
		})
	}
	return nil
}

func (t *TestAdtDiff) Remove(val *typegen.Deferred) error {
	v := new(runtime.CBORBytes)
	err := v.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	t.Removed = append(t.Removed, *v)
	return nil
}

func newContextStore() *contextStore {
	ctx := context.Background()
	bs := bstore.NewBlockstore(ds_sync.MutexWrap(ds.NewMapDatastore()))
	store := cbornode.NewCborStore(bs)
	return &contextStore{
		ctx: ctx,
		cst: store,
	}
}
