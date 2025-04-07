package v2api

import (
	"context"

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
}
