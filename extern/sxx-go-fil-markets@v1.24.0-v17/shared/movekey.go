package shared

import (
	"context"

	"github.com/ipfs/go-datastore"
)

// MoveKey moves a key in a data store
func MoveKey(ds datastore.Datastore, old string, new string) error {
	ctx := context.TODO()
	oldKey := datastore.NewKey(old)
	newKey := datastore.NewKey(new)
	has, err := ds.Has(ctx, oldKey)
	if err != nil {
		return err
	}
	if !has {
		return nil
	}
	value, err := ds.Get(ctx, oldKey)
	if err != nil {
		return err
	}
	err = ds.Put(ctx, newKey, value)
	if err != nil {
		return err
	}
	err = ds.Delete(ctx, oldKey)
	if err != nil {
		return err
	}
	return nil
}
