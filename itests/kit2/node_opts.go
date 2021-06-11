package kit2

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/node"
)

// DefaultPresealsPerBootstrapMiner is the number of preseals that every
// bootstrap miner has by default. It can be overridden through the
// PresealSectors option.
const DefaultPresealsPerBootstrapMiner = 2

// nodeOpts is an options accumulating struct, where functional options are
// merged into.
type nodeOpts struct {
	balance       abi.TokenAmount
	lite          bool
	sectors       int
	rpc           bool
	ownerKey      *wallet.Key
	extraNodeOpts []node.Option
}

// DefaultNodeOpts are the default options that will be applied to test nodes.
var DefaultNodeOpts = nodeOpts{
	balance: big.Mul(big.NewInt(100000000), types.NewInt(build.FilecoinPrecision)),
	sectors: DefaultPresealsPerBootstrapMiner,
}

// NodeOpt is a functional option for test nodes.
type NodeOpt func(opts *nodeOpts) error

// OwnerBalance specifies the balance to be attributed to a miner's owner
// account. Only relevant when creating a miner.
func OwnerBalance(balance abi.TokenAmount) NodeOpt {
	return func(opts *nodeOpts) error {
		opts.balance = balance
		return nil
	}
}

// LiteNode specifies that this node will be a lite node. Only relevant when
// creating a fullnode.
func LiteNode() NodeOpt {
	return func(opts *nodeOpts) error {
		opts.lite = true
		return nil
	}
}

// PresealSectors specifies the amount of preseal sectors to give to a miner
// at genesis. Only relevant when creating a miner.
func PresealSectors(sectors int) NodeOpt {
	return func(opts *nodeOpts) error {
		opts.sectors = sectors
		return nil
	}
}

// ThroughRPC makes interactions with this node throughout the test flow through
// the JSON-RPC API.
func ThroughRPC() NodeOpt {
	return func(opts *nodeOpts) error {
		opts.rpc = true
		return nil
	}
}

// OwnerAddr sets the owner address of a miner. Only relevant when creating
// a miner.
func OwnerAddr(wk *wallet.Key) NodeOpt {
	return func(opts *nodeOpts) error {
		opts.ownerKey = wk
		return nil
	}
}

// ConstructorOpts are Lotus node constructor options that are passed as-is to
// the node.
func ConstructorOpts(extra ...node.Option) NodeOpt {
	return func(opts *nodeOpts) error {
		opts.extraNodeOpts = extra
		return nil
	}
}
