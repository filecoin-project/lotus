package genesis

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	init10 "github.com/filecoin-project/go-state-types/builtin/v10/init"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/vm"
)

func SetupFEVM(ctx context.Context, cs *store.ChainStore, sys vm.SyscallBuilder, sroot cid.Cid, nv network.Version) (cid.Cid, error) {
	av, err := actorstypes.VersionForNetwork(nv)
	if err != nil {
		return cid.Undef, fmt.Errorf("failed to get actors version for network version %d: %w", nv, err)
	}

	if av < actorstypes.Version10 {
		// Not defined before version 10; migration has to setup.
		return sroot, nil
	}

	csc := func(context.Context, abi.ChainEpoch, *state.StateTree) (abi.TokenAmount, error) {
		return big.Zero(), nil
	}

	newVM := func(base cid.Cid) (vm.Interface, error) {
		vmopt := &vm.VMOpts{
			StateBase:      base,
			Epoch:          0,
			Rand:           &fakeRand{},
			Bstore:         cs.StateBlockstore(),
			Actors:         filcns.NewActorRegistry(),
			Syscalls:       mkFakedSigSyscalls(sys),
			CircSupplyCalc: csc,
			NetworkVersion: nv,
			BaseFee:        big.Zero(),
		}

		return vm.NewVM(ctx, vmopt)
	}

	genesisVm, err := newVM(sroot)
	if err != nil {
		return cid.Undef, fmt.Errorf("creating vm: %w", err)
	}

	// The ETH0 address is occupied by an empty contract EVM actor
	evmCodeCid, ok := actors.GetActorCodeID(av, actors.EvmKey)
	if !ok {
		return cid.Undef, fmt.Errorf("failed to get CodeCID for EVM during genesis")
	}

	eth0Addr, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, make([]byte, 32))
	if err != nil {
		return cid.Undef, fmt.Errorf("failed to create ETH0 f4 address: %w", err)
	}

	eth0AddrBytes, err := eth0Addr.Marshal()
	if err != nil {
		return cid.Undef, fmt.Errorf("failed to marshal ETH0 f4 address: %w", err)
	}

	params := &init10.Exec4Params{
		CodeCID:           evmCodeCid,
		ConstructorParams: []byte{},
		SubAddress:        eth0AddrBytes,
	}

	// TODO method 3 is Exec4; we have to name the methods in go-state-types and avoid using the number
	//      directly.
	if _, err := doExecValue(ctx, genesisVm, builtintypes.InitActorAddr, builtintypes.EthereumAddressManagerActorAddr, big.Zero(), 3, mustEnc(params)); err != nil {
		return cid.Undef, fmt.Errorf("creating ETH0 actor: %w", err)
	}

	newroot, err := genesisVm.Flush(ctx)
	if err != nil {
		return cid.Undef, fmt.Errorf("flushing vm: %w", err)
	}

	return newroot, nil

}
