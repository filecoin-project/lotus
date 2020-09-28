package adt

import (
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/actors"

	adt0 "github.com/filecoin-project/specs-actors/actors/util/adt"
	adt2 "github.com/filecoin-project/specs-actors/v2/actors/util/adt"
)

type Map interface {
	Root() (cid.Cid, error)

	Put(k abi.Keyer, v cbor.Marshaler) error
	Get(k abi.Keyer, v cbor.Unmarshaler) (bool, error)
	Delete(k abi.Keyer) error

	ForEach(v cbor.Unmarshaler, fn func(key string) error) error
}

func AsMap(store Store, root cid.Cid, version actors.Version) (Map, error) {
	switch version {
	case actors.Version0:
		return adt0.AsMap(store, root)
	case actors.Version2:
		return adt2.AsMap(store, root)
	}
	return nil, xerrors.Errorf("unknown network version: %d", version)
}

func NewMap(store Store, version actors.Version) (Map, error) {
	switch version {
	case actors.Version0:
		return adt0.MakeEmptyMap(store), nil
	case actors.Version2:
		return adt2.MakeEmptyMap(store), nil
	}
	return nil, xerrors.Errorf("unknown network version: %d", version)
}

type Array interface {
	Root() (cid.Cid, error)

	Set(idx uint64, v cbor.Marshaler) error
	Get(idx uint64, v cbor.Unmarshaler) (bool, error)
	Delete(idx uint64) error
	Length() uint64

	ForEach(v cbor.Unmarshaler, fn func(idx int64) error) error
}

func AsArray(store Store, root cid.Cid, version network.Version) (Array, error) {
	switch actors.VersionForNetwork(version) {
	case actors.Version0:
		return adt0.AsArray(store, root)
	case actors.Version2:
		return adt2.AsArray(store, root)
	}
	return nil, xerrors.Errorf("unknown network version: %d", version)
}

func NewArray(store Store, version actors.Version) (Array, error) {
	switch version {
	case actors.Version0:
		return adt0.MakeEmptyArray(store), nil
	case actors.Version2:
		return adt2.MakeEmptyArray(store), nil
	}
	return nil, xerrors.Errorf("unknown network version: %d", version)
}
