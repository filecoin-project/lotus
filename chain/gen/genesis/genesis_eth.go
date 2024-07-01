package genesis

import (
	"context"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/manifest"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/chain/vm"
)

// EthNullAddresses are the Ethereum addresses we want to create zero-balanced EthAccounts in.
// We may want to add null addresses for precompiles going forward.
var EthNullAddresses = []string{
	"0x0000000000000000000000000000000000000000",
}

func SetupEAM(_ context.Context, nst *state.StateTree, nv network.Version) error {
	av, err := actorstypes.VersionForNetwork(nv)
	if err != nil {
		return fmt.Errorf("failed to get actors version for network version %d: %w", nv, err)
	}

	if av < actorstypes.Version10 {
		// Not defined before version 10; migration has to create.
		return nil
	}

	codecid, ok := actors.GetActorCodeID(av, manifest.EamKey)
	if !ok {
		return fmt.Errorf("failed to get CodeCID for EAM during genesis")
	}

	header := &types.Actor{
		Code:    codecid,
		Head:    vm.EmptyObjectCid,
		Balance: big.Zero(),
	}
	return nst.SetActor(builtin.EthereumAddressManagerActorAddr, header)
}

// MakeEthNullAddressActor creates a null address actor at the specified Ethereum address.
func MakeEthNullAddressActor(av actorstypes.Version, addr address.Address) (*types.Actor, error) {
	actcid, ok := actors.GetActorCodeID(av, manifest.EthAccountKey)
	if !ok {
		return nil, xerrors.Errorf("failed to get EthAccount actor code ID for actors version %d", av)
	}

	if addr.Protocol() != address.Delegated {
		return nil, xerrors.Errorf("eth accounts must have f4 addresses, %s is not an f4 address", addr)
	}

	act := &types.Actor{
		Code:             actcid,
		Head:             vm.EmptyObjectCid,
		Nonce:            0,
		Balance:          big.Zero(),
		DelegatedAddress: &addr,
	}

	return act, nil
}

func SetupEthNullAddresses(ctx context.Context, st *state.StateTree, nv network.Version) ([]address.Address, error) {
	av, err := actorstypes.VersionForNetwork(nv)
	if err != nil {
		return nil, xerrors.Errorf("failed to resolve actors version for network version %d: %w", av, err)
	}

	if av < actorstypes.Version10 {
		// Not defined before version 10.
		return nil, nil
	}

	var ethAddresses []ethtypes.EthAddress
	for _, addr := range EthNullAddresses {
		a, err := ethtypes.ParseEthAddress(addr)
		if err != nil {
			return nil, xerrors.Errorf("failed to represent the 0x0 as an EthAddress: %w", err)
		}
		ethAddresses = append(ethAddresses, a)
	}

	initAct, err := st.GetActor(builtin.InitActorAddr)
	if err != nil {
		return nil, xerrors.Errorf("failed to load init actor: %w", err)
	}

	initState, err := init_.Load(adt.WrapStore(ctx, st.Store), initAct)
	if err != nil {
		return nil, xerrors.Errorf("failed to load init actor state: %w", err)
	}

	var ret []address.Address
	for _, ethAddr := range ethAddresses {
		// Place an EthAccount at the 0x0 Eth Null Address.
		f4Addr, err := ethAddr.ToFilecoinAddress()
		if err != nil {
			return nil, xerrors.Errorf("failed to compute Filecoin address for Eth addr 0x0: %w", err)
		}

		idAddr, err := initState.MapAddressToNewID(f4Addr)
		if err != nil {
			return nil, xerrors.Errorf("failed to map addr in init actor: %w", err)
		}

		actState, err := MakeEthNullAddressActor(av, f4Addr)
		if err != nil {
			return nil, xerrors.Errorf("failed to create EthAccount actor for null address: %w", err)
		}

		if err := st.SetActor(idAddr, actState); err != nil {
			return nil, xerrors.Errorf("failed to set Eth Null Address EthAccount actor state: %w", err)
		}

		ret = append(ret, idAddr)
	}

	initAct.Head, err = st.Store.Put(ctx, initState)
	if err != nil {
		return nil, xerrors.Errorf("failed to add init actor state to store: %w", err)
	}

	if err := st.SetActor(builtin.InitActorAddr, initAct); err != nil {
		return nil, xerrors.Errorf("failed to set updated state for init actor: %w", err)
	}

	return ret, nil
}
