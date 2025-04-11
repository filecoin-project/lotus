package v2api

import (
	"context"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/chain/types"
)

//go:generate go run github.com/golang/mock/mockgen -destination=v2mocks/mock_full.go -package=v2mocks . FullNode

// FullNode represents an interface for the v2 full node APIs. This interface
// currently consists of chain-related functionalities and the API is
// experimental and subject to change.
type FullNode interface {
	// MethodGroup: Chain
	// The Chain method group contains methods for interacting with
	// the blockchain.
	//
	// <b>Note: This API is experimental and may change in the future.<b/>
	//
	// Please see Filecoin V2 API design documentation for more details:
	//   - https://www.notion.so/filecoindev/Lotus-F3-aware-APIs-1cfdc41950c180ae97fef580e79427d5
	//   - https://www.notion.so/filecoindev/Filecoin-V2-APIs-1d0dc41950c1808b914de5966d501658

	// ChainGetTipSet retrieves a tipset that corresponds to the specified selector
	// criteria. The criteria can be provided in the form of a tipset key, a
	// blockchain height including an optional fallback to previous non-null tipset,
	// or a designated tag such as "latest" or "finalized".
	//
	// The "Finalized" tag returns the tipset that is considered finalized based on
	// the consensus protocol of the current node, either Filecoin EC Finality or
	// Filecoin Fast Finality (F3). The finalized tipset selection gracefully falls
	// back to EC finality in cases where F3 isn't ready or not running.
	//
	// In a case where no selector is provided, an error is returned. The selector
	// must be explicitly specified.
	//
	// For more details, refer to the types.TipSetSelector and
	// types.NewTipSetSelector.
	//
	// Example usage:
	//
	//	selector := types.TipSetSelectors.Latest
	//	tipSet, err := node.ChainGetTipSet(context.Background(), selector)
	//	if err != nil {
	//		fmt.Println("Error retrieving tipset:", err)
	//		return
	//	}
	//	fmt.Printf("Latest TipSet: %v\n", tipSet)
	//
	ChainGetTipSet(context.Context, types.TipSetSelector) (*types.TipSet, error) //perm:read

	// MethodGroup: State
	// The State method group contains methods for interacting with the Filecoin
	// blockchain state, including actor information, addresses, and chain data.
	// These methods allow querying the blockchain state at any point in its history
	// using flexible TipSet selection mechanisms.

	// StateGetActor retrieves the actor information for the specified address at the
	// selected tipset.
	//
	// This function returns the on-chain Actor object including:
	//   - Code CID (determines the actor's type)
	//   - State root CID
	//   - Balance in attoFIL
	//   - Nonce (for account actors)
	//
	// The TipSetSelector parameter provides flexible options for selecting the tipset:
	//   - TipSetSelectors.Latest: the most recent tipset with the heaviest weight
	//   - TipSetSelectors.Finalized: the most recent finalized tipset
	//   - TipSetSelectors.Height(epoch, previous, anchor): tipset at the specified height
	//   - TipSetSelectors.Key(key): tipset with the specified key
	//
	// See types.TipSetSelector documentation for additional details.
	//
	// If the actor does not exist at the specified tipset, this function returns nil.
	//
	// Experimental: This API is experimental and may change without notice.
	StateGetActor(context.Context, address.Address, types.TipSetSelector) (*types.Actor, error) //perm:read

	// StateGetID retrieves the ID address for the specified address at the selected tipset.
	//
	// Every actor on the Filecoin network has a unique ID address (format: f0123).
	// This function resolves any address type (ID, robust, or delegated) to its canonical
	// ID address representation at the specified tipset.
	//
	// The function is particularly useful for:
	//   - Normalizing different address formats to a consistent representation
	//   - Following address changes across state transitions
	//   - Verifying that an address corresponds to an existing actor
	//
	// The TipSetSelector parameter provides flexible options for selecting the tipset.
	// See StateGetActor documentation for details on selection options.
	//
	// If the address cannot be resolved at the specified tipset, this function returns nil.
	//
	// Experimental: This API is experimental and may change without notice.
	StateGetID(context.Context, address.Address, types.TipSetSelector) (*address.Address, error) //perm:read
}
