package adt

import (
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	v0adt "github.com/filecoin-project/specs-actors/actors/util/adt"
)

type Map interface {
	Root() (cid.Cid, error)

	Put(k abi.Keyer, v cbor.Marshaler) error
	Get(k abi.Keyer, v cbor.Unmarshaler) (bool, error)
	Delete(k abi.Keyer) error

	ForEach(v cbor.Unmarshaler, fn func(key string) error) error
}

func AsMap(store Store, root cid.Cid, version network.Version) (Map, error) {
	switch builtin.VersionForNetwork(version) {
	case builtin.Version0:
		return v0adt.AsMap(store, root)
	}
	return nil, xerrors.Errorf("unknown network version: %d", version)
}

func NewMap(store Store, version network.Version) (Map, error) {
	switch builtin.VersionForNetwork(version) {
	case builtin.Version0:
		return v0adt.MakeEmptyMap(store)
	}
	return nil, xerrors.Errorf("unknown network version: %d", version)
}

type Array interface {
	Root() (cid.Cid, error)

	Set(idx uint64, v cbor.Marshaler) error
	Get(idx uint64, v cbor.Unmarshaler) (bool, error)
	Delete(idx uint64) error
	Length() uint64

	ForEach(v cbor.Unmarshaler, fn func(idx int) error) error
}

func AsArray(store Store, root cid.Cid, version network.Version) (Array, error) {
	switch builtin.VersionForNetwork(version) {
	case builtin.Version0:
		return v0adt.AsArray(store, root)
	}
	return nil, xerrors.Errorf("unknown network version: %d", version)
}
