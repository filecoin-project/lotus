package init

import (
	"crypto/sha256"
	"fmt"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	builtin14 "github.com/filecoin-project/go-state-types/builtin"
	init14 "github.com/filecoin-project/go-state-types/builtin/v14/init"
	adt14 "github.com/filecoin-project/go-state-types/builtin/v14/util/adt"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var _ State = (*state14)(nil)

func load14(store adt.Store, root cid.Cid) (State, error) {
	out := state14{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make14(store adt.Store, networkName string) (State, error) {
	out := state14{store: store}

	s, err := init14.ConstructState(store, networkName)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state14 struct {
	init14.State
	store adt.Store
}

func (s *state14) ResolveAddress(address address.Address) (address.Address, bool, error) {
	return s.State.ResolveAddress(s.store, address)
}

func (s *state14) MapAddressToNewID(address address.Address) (address.Address, error) {
	return s.State.MapAddressToNewID(s.store, address)
}

func (s *state14) ForEachActor(cb func(id abi.ActorID, address address.Address) error) error {
	addrs, err := adt14.AsMap(s.store, s.State.AddressMap, builtin14.DefaultHamtBitwidth)
	if err != nil {
		return err
	}
	var actorID cbg.CborInt
	return addrs.ForEach(&actorID, func(key string) error {
		addr, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(abi.ActorID(actorID), addr)
	})
}

func (s *state14) NetworkName() (dtypes.NetworkName, error) {
	return dtypes.NetworkName(s.State.NetworkName), nil
}

func (s *state14) SetNetworkName(name string) error {
	s.State.NetworkName = name
	return nil
}

func (s *state14) SetNextID(id abi.ActorID) error {
	s.State.NextID = id
	return nil
}

func (s *state14) Remove(addrs ...address.Address) (err error) {
	m, err := adt14.AsMap(s.store, s.State.AddressMap, builtin14.DefaultHamtBitwidth)
	if err != nil {
		return err
	}
	for _, addr := range addrs {
		if err = m.Delete(abi.AddrKey(addr)); err != nil {
			return xerrors.Errorf("failed to delete entry for address: %s; err: %w", addr, err)
		}
	}
	amr, err := m.Root()
	if err != nil {
		return xerrors.Errorf("failed to get address map root: %w", err)
	}
	s.State.AddressMap = amr
	return nil
}

func (s *state14) SetAddressMap(mcid cid.Cid) error {
	s.State.AddressMap = mcid
	return nil
}

func (s *state14) GetState() interface{} {
	return &s.State
}

func (s *state14) AddressMap() (adt.Map, error) {
	return adt14.AsMap(s.store, s.State.AddressMap, builtin14.DefaultHamtBitwidth)
}

func (s *state14) AddressMapBitWidth() int {
	return builtin14.DefaultHamtBitwidth
}

func (s *state14) AddressMapHashFunction() func(input []byte) []byte {
	return func(input []byte) []byte {
		res := sha256.Sum256(input)
		return res[:]
	}
}

func (s *state14) ActorKey() string {
	return manifest.InitKey
}

func (s *state14) ActorVersion() actorstypes.Version {
	return actorstypes.Version14
}

func (s *state14) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
