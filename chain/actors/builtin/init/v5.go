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
	"github.com/filecoin-project/go-state-types/manifest"
	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	init5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/init"
	adt5 "github.com/filecoin-project/specs-actors/v5/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var _ State = (*state5)(nil)

func load5(store adt.Store, root cid.Cid) (State, error) {
	out := state5{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make5(store adt.Store, networkName string) (State, error) {
	out := state5{store: store}

	s, err := init5.ConstructState(store, networkName)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state5 struct {
	init5.State
	store adt.Store
}

func (s *state5) ResolveAddress(address address.Address) (address.Address, bool, error) {
	return s.State.ResolveAddress(s.store, address)
}

func (s *state5) MapAddressToNewID(address address.Address) (address.Address, error) {
	return s.State.MapAddressToNewID(s.store, address)
}

func (s *state5) ForEachActor(cb func(id abi.ActorID, address address.Address) error) error {
	addrs, err := adt5.AsMap(s.store, s.State.AddressMap, builtin5.DefaultHamtBitwidth)
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

func (s *state5) NetworkName() (dtypes.NetworkName, error) {
	return dtypes.NetworkName(s.State.NetworkName), nil
}

func (s *state5) SetNetworkName(name string) error {
	s.State.NetworkName = name
	return nil
}

func (s *state5) SetNextID(id abi.ActorID) error {
	s.State.NextID = id
	return nil
}

func (s *state5) Remove(addrs ...address.Address) (err error) {
	m, err := adt5.AsMap(s.store, s.State.AddressMap, builtin5.DefaultHamtBitwidth)
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

func (s *state5) SetAddressMap(mcid cid.Cid) error {
	s.State.AddressMap = mcid
	return nil
}

func (s *state5) GetState() interface{} {
	return &s.State
}

func (s *state5) AddressMap() (adt.Map, error) {
	return adt5.AsMap(s.store, s.State.AddressMap, builtin5.DefaultHamtBitwidth)
}

func (s *state5) AddressMapBitWidth() int {
	return builtin5.DefaultHamtBitwidth
}

func (s *state5) AddressMapHashFunction() func(input []byte) []byte {
	return func(input []byte) []byte {
		res := sha256.Sum256(input)
		return res[:]
	}
}

func (s *state5) ActorKey() string {
	return manifest.InitKey
}

func (s *state5) ActorVersion() actorstypes.Version {
	return actorstypes.Version5
}

func (s *state5) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
