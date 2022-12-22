package store

import (
	"errors"
	"hlm-ipfs/x/store"

	"github.com/ipfs/go-datastore"

	"github.com/google/uuid"
)

const (
	prefix = "lotus-x:"
)

func Init(repo string) error {
	return store.Init(repo)
}

func WorkerID() (uuid.UUID, error) {
	var (
		err  error
		data []byte
		id   = uuid.New()
		nop  = [16]byte{}
		key  = prefix + "worker:id"
	)
	if data, err = store.Get(key); err != nil {
		if !errors.Is(err, datastore.ErrNotFound) {
			return nop, err
		}
	} else if id, err = uuid.ParseBytes(data); err != nil {
		return nop, err
	}
	if err = store.Set(key, []byte(id.String())); err != nil {
		return nop, err
	}
	return id, nil
}
