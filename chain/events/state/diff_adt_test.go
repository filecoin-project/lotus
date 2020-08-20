package state

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cbornode "github.com/ipfs/go-ipld-cbor"
	typegen "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	bstore "github.com/filecoin-project/lotus/lib/blockstore"
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

	changes := new(TestDiffArray)

	assert.NoError(t, DiffAdtArray(arrA, arrB, changes))
	assert.NotNil(t, changes)

	assert.Equal(t, 2, len(changes.Added))
	// keys 5 and 6 were added
	assert.EqualValues(t, uint64(5), changes.Added[0].key)
	assert.EqualValues(t, []byte{8}, changes.Added[0].val)
	assert.EqualValues(t, uint64(6), changes.Added[1].key)
	assert.EqualValues(t, []byte{9}, changes.Added[1].val)

	assert.Equal(t, 2, len(changes.Modified))
	// keys 1 and 4 were modified
	assert.EqualValues(t, uint64(1), changes.Modified[0].From.key)
	assert.EqualValues(t, []byte{0}, changes.Modified[0].From.val)
	assert.EqualValues(t, uint64(1), changes.Modified[0].To.key)
	assert.EqualValues(t, []byte{1}, changes.Modified[0].To.val)
	assert.EqualValues(t, uint64(4), changes.Modified[1].From.key)
	assert.EqualValues(t, []byte{0}, changes.Modified[1].From.val)
	assert.EqualValues(t, uint64(4), changes.Modified[1].To.key)
	assert.EqualValues(t, []byte{6}, changes.Modified[1].To.val)

	assert.Equal(t, 2, len(changes.Removed))
	// keys 0 and 2 were deleted
	assert.EqualValues(t, uint64(0), changes.Removed[0].key)
	assert.EqualValues(t, []byte{0}, changes.Removed[0].val)
	assert.EqualValues(t, uint64(2), changes.Removed[1].key)
	assert.EqualValues(t, []byte{1}, changes.Removed[1].val)
}

func TestDiffAdtMap(t *testing.T) {
	ctxstoreA := newContextStore()
	ctxstoreB := newContextStore()

	mapA := adt.MakeEmptyMap(ctxstoreA)
	mapB := adt.MakeEmptyMap(ctxstoreB)

	require.NoError(t, mapA.Put(adt.UIntKey(0), runtime.CBORBytes([]byte{0}))) // delete

	require.NoError(t, mapA.Put(adt.UIntKey(1), runtime.CBORBytes([]byte{0}))) // modify
	require.NoError(t, mapB.Put(adt.UIntKey(1), runtime.CBORBytes([]byte{1})))

	require.NoError(t, mapA.Put(adt.UIntKey(2), runtime.CBORBytes([]byte{1}))) // delete

	require.NoError(t, mapA.Put(adt.UIntKey(3), runtime.CBORBytes([]byte{0}))) // noop
	require.NoError(t, mapB.Put(adt.UIntKey(3), runtime.CBORBytes([]byte{0})))

	require.NoError(t, mapA.Put(adt.UIntKey(4), runtime.CBORBytes([]byte{0}))) // modify
	require.NoError(t, mapB.Put(adt.UIntKey(4), runtime.CBORBytes([]byte{6})))

	require.NoError(t, mapB.Put(adt.UIntKey(5), runtime.CBORBytes{8})) // add
	require.NoError(t, mapB.Put(adt.UIntKey(6), runtime.CBORBytes{9})) // add

	changes := new(TestDiffMap)

	assert.NoError(t, DiffAdtMap(mapA, mapB, changes))
	assert.NotNil(t, changes)

	assert.Equal(t, 2, len(changes.Added))
	// keys 5 and 6 were added
	assert.EqualValues(t, uint64(6), changes.Added[0].key)
	assert.EqualValues(t, []byte{9}, changes.Added[0].val)
	assert.EqualValues(t, uint64(5), changes.Added[1].key)
	assert.EqualValues(t, []byte{8}, changes.Added[1].val)

	assert.Equal(t, 2, len(changes.Modified))
	// keys 1 and 4 were modified
	assert.EqualValues(t, uint64(1), changes.Modified[0].From.key)
	assert.EqualValues(t, []byte{0}, changes.Modified[0].From.val)
	assert.EqualValues(t, uint64(1), changes.Modified[0].To.key)
	assert.EqualValues(t, []byte{1}, changes.Modified[0].To.val)
	assert.EqualValues(t, uint64(4), changes.Modified[1].From.key)
	assert.EqualValues(t, []byte{0}, changes.Modified[1].From.val)
	assert.EqualValues(t, uint64(4), changes.Modified[1].To.key)
	assert.EqualValues(t, []byte{6}, changes.Modified[1].To.val)

	assert.Equal(t, 2, len(changes.Removed))
	// keys 0 and 2 were deleted
	assert.EqualValues(t, uint64(0), changes.Removed[0].key)
	assert.EqualValues(t, []byte{0}, changes.Removed[0].val)
	assert.EqualValues(t, uint64(2), changes.Removed[1].key)
	assert.EqualValues(t, []byte{1}, changes.Removed[1].val)

}

type TestDiffMap struct {
	Added    []adtMapDiffResult
	Modified []TestAdtMapDiffModified
	Removed  []adtMapDiffResult
}

var _ AdtMapDiff = &TestDiffMap{}

func (t *TestDiffMap) AsKey(key string) (adt.Keyer, error) {
	k, err := adt.ParseUIntKey(key)
	if err != nil {
		return nil, err
	}
	return adt.UIntKey(k), nil
}

func (t *TestDiffMap) Add(key string, val *typegen.Deferred) error {
	v := new(runtime.CBORBytes)
	err := v.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	k, err := adt.ParseUIntKey(key)
	if err != nil {
		return err
	}
	t.Added = append(t.Added, adtMapDiffResult{
		key: k,
		val: *v,
	})
	return nil
}

func (t *TestDiffMap) Modify(key string, from, to *typegen.Deferred) error {
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

	k, err := adt.ParseUIntKey(key)
	if err != nil {
		return err
	}

	if !bytes.Equal(*vFrom, *vTo) {
		t.Modified = append(t.Modified, TestAdtMapDiffModified{
			From: adtMapDiffResult{
				key: k,
				val: *vFrom,
			},
			To: adtMapDiffResult{
				key: k,
				val: *vTo,
			},
		})
	}
	return nil
}

func (t *TestDiffMap) Remove(key string, val *typegen.Deferred) error {
	v := new(runtime.CBORBytes)
	err := v.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	k, err := adt.ParseUIntKey(key)
	if err != nil {
		return err
	}
	t.Removed = append(t.Removed, adtMapDiffResult{
		key: k,
		val: *v,
	})
	return nil
}

type adtMapDiffResult struct {
	key uint64
	val runtime.CBORBytes
}

type TestAdtMapDiffModified struct {
	From adtMapDiffResult
	To   adtMapDiffResult
}

type adtArrayDiffResult struct {
	key uint64
	val runtime.CBORBytes
}

type TestDiffArray struct {
	Added    []adtArrayDiffResult
	Modified []TestAdtArrayDiffModified
	Removed  []adtArrayDiffResult
}

var _ AdtArrayDiff = &TestDiffArray{}

type TestAdtArrayDiffModified struct {
	From adtArrayDiffResult
	To   adtArrayDiffResult
}

func (t *TestDiffArray) Add(key uint64, val *typegen.Deferred) error {
	v := new(runtime.CBORBytes)
	err := v.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	t.Added = append(t.Added, adtArrayDiffResult{
		key: key,
		val: *v,
	})
	return nil
}

func (t *TestDiffArray) Modify(key uint64, from, to *typegen.Deferred) error {
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
		t.Modified = append(t.Modified, TestAdtArrayDiffModified{
			From: adtArrayDiffResult{
				key: key,
				val: *vFrom,
			},
			To: adtArrayDiffResult{
				key: key,
				val: *vTo,
			},
		})
	}
	return nil
}

func (t *TestDiffArray) Remove(key uint64, val *typegen.Deferred) error {
	v := new(runtime.CBORBytes)
	err := v.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	t.Removed = append(t.Removed, adtArrayDiffResult{
		key: key,
		val: *v,
	})
	return nil
}

func newContextStore() *contextStore {
	ctx := context.Background()
	bs := bstore.NewTemporarySync()
	store := cbornode.NewCborStore(bs)
	return &contextStore{
		ctx: ctx,
		cst: store,
	}
}
