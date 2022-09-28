package mir

import (
	"fmt"
	"strings"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	t "github.com/filecoin-project/mir/pkg/types"
)

func newMirID(subnet, addr string) string {
	return fmt.Sprintf("%s:%s", subnet, addr)
}

// getSubnetValidators retrieves subnet validators from the environment variable or from the state.
// func getSubnetValidators(
// 	ctx context.Context,
// 	api v1api.FullNode,
// ) (*hierarchical.ValidatorSet, error) {
// var err error
// var validators []hierarchical.Validator
// validatorsEnv := os.Getenv(ValidatorsEnv)
// if validatorsEnv != "" {
// 	validators, err = hierarchical.ValidatorsFromString(validatorsEnv)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get validators from string: %w", err)
// 	}
// } else {
// 	if subnetID == address.RootSubnet {
// 		return nil, fmt.Errorf("can't be run in rootnet without validators")
// 	}
// 	validators, err = api.SubnetStateGetValidators(ctx, subnetID)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get validators from state")
// 	}
// }
// return hierarchical.NewValidatorSet(validators), nil
// 	panic("not implemented")
// }

// validatorsMembership validates that validators addresses are valid multi-addresses and
// returns all validators IDs and map between IDs and multi-addresses.
// func validatorsMembership(validators []hierarchical.Validator) ([]t.NodeID, map[t.NodeID]t.NodeAddress, error) {
// 	var nodeIDs []t.NodeID
// 	nodeAddrs := make(map[t.NodeID]t.NodeAddress)

// 	for _, v := range validators {
// 		id := t.NodeID(v.ID())
// 		a, err := multiaddr.NewMultiaddr(v.NetAddr)
// 		if err != nil {
// 			return nil, nil, err
// 		}
// 		nodeIDs = append(nodeIDs, id)
// 		nodeAddrs[id] = a
// 	}

// 	return nodeIDs, nodeAddrs, nil
// }

// getBlockMiner computes the miner address for the block at h.
func getBlockMiner(validators []t.NodeID, h abi.ChainEpoch) (address.Address, error) {
	addr := validators[int(h)%len(validators)]
	a, err := address.NewFromString(strings.Split(addr.Pb(), ":")[1])
	if err != nil {
		return address.Undef, err
	}
	return a, nil
}
