package kit

import (
	"context"

	"github.com/filecoin-project/lotus/chain/ecfinality"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/impl/eth"
)

// MockECFinalityProvider satisfies ecfinality.Provider (for ChainModuleV2) and
// eth.ECFinalityProvider (for tipSetResolver), allowing tests to control the
// EC finality calculator response without the additional cost of running the
// FRC-0089 calculator across 905 tipsets on every head change when the block
// time is tiny.
//
// Set FinalizedTipSet and ThresholdDepth to control the response. Set Err to
// simulate a calculator failure.
type MockECFinalityProvider struct {
	FinalizedTipSet *types.TipSet
	ThresholdDepth  int
	Err             error
}

func NewMockECFinalityProvider() *MockECFinalityProvider {
	return &MockECFinalityProvider{
		ThresholdDepth: -1,
	}
}

func (m *MockECFinalityProvider) GetFinalizedTipSet(_ context.Context) (*types.TipSet, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.FinalizedTipSet, nil
}

func (m *MockECFinalityProvider) GetStatus(_ context.Context) (*ecfinality.ECFinalityStatus, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return &ecfinality.ECFinalityStatus{
		ThresholdDepth:  m.ThresholdDepth,
		FinalizedTipSet: m.FinalizedTipSet,
	}, nil
}

var (
	_ ecfinality.Provider    = (*MockECFinalityProvider)(nil)
	_ eth.ECFinalityProvider = (*MockECFinalityProvider)(nil)
)

// ECFinalityProvider overrides the EC finality calculator used by the test
// node with the given mock, replacing the real ECFinalityCache which walks
// 905 tipsets on every head change.
func ECFinalityProvider(provider *MockECFinalityProvider) NodeOpt {
	return ConstructorOpts(
		node.Override(new(ecfinality.Provider), provider),
		node.Override(new(eth.ECFinalityProvider), provider),
	)
}
