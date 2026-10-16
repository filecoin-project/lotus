package cli

import (
	"context"
	"io"
	"os"
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
	app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("state", StateGetDealSetCmd))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// A deal label is set by the deal's client and only has to be valid UTF-8,
	// so it can carry C1 control characters (U+0080-U+009F). U+009B is the CSI
	// control that begins an ANSI escape sequence on many terminals, which JSON
	// encoding leaves raw.
	label, err := markettypes.NewLabelFromString("deal\u009b2K\u009b31m")
	require.NoError(t, err)

	ts := mock.TipSet(mock.MkBlock(nil, 0, 0))
	deal := &api.MarketDeal{
		Proposal: market.DealProposal{
			PieceSize:            abi.PaddedPieceSize(2048),
			Client:               mock.Address(1),
			Provider:             mock.Address(2),
			Label:                label,
			StoragePricePerEpoch: big.Zero(),
			ProviderCollateral:   big.Zero(),
			ClientCollateral:     big.Zero(),
		},
		State: api.MarketDealState{
			SectorStartEpoch: -1,
			LastUpdatedEpoch: -1,
			SlashEpoch:       -1,
		},
	}

	gomock.InOrder(
		mockApi.EXPECT().ChainHead(ctx).Return(ts, nil),
		mockApi.EXPECT().StateMarketStorageDeal(ctx, abi.DealID(1), ts.Key()).Return(deal, nil),
	)

	// get-deal prints the deal to stdout, so capture it directly.
	out := captureStdout(t, func() {
		require.NoError(t, app.Run([]string{"state", "get-deal", "1"}))
	})

	require.NotContains(t, out, "\u009b", "raw C1 control leaked to terminal output")
	require.Contains(t, out, `\u009b`, "label C1 control should be rendered as an escape")
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	require.NoError(t, w.Close())
	data, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(data)
}
