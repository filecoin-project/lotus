package eth

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"

	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

var _ EthFilecoinAPI = (*ethFilecoin)(nil)

type ethFilecoin struct {
	stateManager StateManager

	tipsetResolver TipSetResolver
}

func NewEthFilecoinAPI(stateManager StateManager, tipsetResolver TipSetResolver) EthFilecoinAPI {
	return &ethFilecoin{
		stateManager:   stateManager,
		tipsetResolver: tipsetResolver,
	}
}

func (e *ethFilecoin) EthAddressToFilecoinAddress(ctx context.Context, ethAddress ethtypes.EthAddress) (address.Address, error) {
	return ethAddress.ToFilecoinAddress()
}

func (e *ethFilecoin) FilecoinAddressToEthAddress(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthAddress, error) {
	params, err := jsonrpc.DecodeParams[ethtypes.FilecoinAddressToEthAddressParams](p)
	if err != nil {
		return ethtypes.EthAddress{}, xerrors.Errorf("decoding params: %w", err)
	}

	filecoinAddress := params.FilecoinAddress

	// If the address is an "f0" or "f4" address, `EthAddressFromFilecoinAddress` will return the corresponding Ethereum address right away.
	if eaddr, err := ethtypes.EthAddressFromFilecoinAddress(filecoinAddress); err == nil {
		return eaddr, nil
	} else if err != ethtypes.ErrInvalidAddress {
		return ethtypes.EthAddress{}, xerrors.Errorf("error converting filecoin address to eth address: %w", err)
	}

	// We should only be dealing with "f1"/"f2"/"f3" addresses from here-on.
	switch filecoinAddress.Protocol() {
	case address.SECP256K1, address.Actor, address.BLS:
		// Valid protocols
	default:
		// Ideally, this should never happen but is here for sanity checking.
		return ethtypes.EthAddress{}, xerrors.Errorf("invalid filecoin address protocol: %s", filecoinAddress.String())
	}

	var blkParam string
	if params.BlkParam == nil {
		blkParam = "finalized"
	} else {
		blkParam = *params.BlkParam
	}

	ts, err := e.tipsetResolver.GetTipsetByBlockNumber(ctx, blkParam, false)
	if err != nil {
		return ethtypes.EthAddress{}, err // don't wrap, to preserve ErrNullRound
	}

	// Lookup the ID address
	idAddr, err := e.stateManager.LookupIDAddress(ctx, filecoinAddress, ts)
	if err != nil {
		return ethtypes.EthAddress{}, xerrors.Errorf(
			"failed to lookup ID address for given Filecoin address %s ("+
				"ensure that the address has been instantiated on-chain and sufficient epochs have passed since instantiation to confirm to the given 'blkParam': \"%s\"): %w",
			filecoinAddress,
			blkParam,
			err,
		)
	}

	// Convert the ID address an ETH address
	ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(idAddr)
	if err != nil {
		return ethtypes.EthAddress{}, xerrors.Errorf("failed to convert filecoin ID address %s to eth address: %w", idAddr, err)
	}

	return ethAddr, nil
}
