package cli

import (
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/types/mock"
)

func TestStateGetDealEscapesLabel(t *testing.T) {
	app, mockApi, out, done := NewMockAppWithFullAPI(t, WithCategory("state", StateGetDealSetCmd))
	defer done()

	label, err := markettypes.NewLabelFromString("deal\u009b2K\u009b31m")
	require.NoError(t, err)

	ts := mock.TipSet(mock.MkBlock(nil, 0, 0))
	deal := &api.MarketDeal{
		Proposal: market.DealProposal{
			Client:               mock.Address(1),
			Provider:             mock.Address(2),
			Label:                label,
			StoragePricePerEpoch: big.Zero(),
			ProviderCollateral:   big.Zero(),
			ClientCollateral:     big.Zero(),
		},
	}

	mockApi.EXPECT().ChainHead(gomock.Any()).Return(ts, nil)
	mockApi.EXPECT().StateMarketStorageDeal(gomock.Any(), abi.DealID(1), ts.Key()).Return(deal, nil)

	require.NoError(t, app.Run([]string{"state", "get-deal", "1"}))
	require.NotContains(t, out.String(), "\u009b", "raw C1 control leaked to terminal output")

	var decoded api.MarketDeal
	require.NoError(t, json.Unmarshal(out.Bytes(), &decoded))
	require.Equal(t, label, decoded.Proposal.Label)
}
