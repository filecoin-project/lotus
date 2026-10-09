package cli

import (
	"errors"
	"io"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api/mocks"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
)

func TestValidateBackfillRetry(t *testing.T) {
	app, mockSrvcs, buf, done := newMockApp(t, IndexCmd)
	defer done()
	app.ErrWriter = io.Discard

	full := mocks.NewMockFullNode(gomock.NewController(t))
	mockSrvcs.EXPECT().FullNodeAPI().Return(full)
	mockSrvcs.EXPECT().Close().Return(nil)

	head := mock.MkBlock(nil, 0, 0)
	head.Height = 100
	full.EXPECT().ChainHead(gomock.Any()).Return(mock.TipSet(head), nil)

	valid := func(epoch abi.ChainEpoch) *types.IndexValidation {
		return &types.IndexValidation{Height: epoch}
	}
	transient := errors.New("transient failure")
	persistent := errors.New("persistent failure")
	missing := errors.New("failed to backfill tipset at epoch 10: chain store does not contain data")

	gomock.InOrder(
		full.EXPECT().ChainValidateIndex(gomock.Any(), abi.ChainEpoch(13), true).Return(valid(13), nil),
		full.EXPECT().ChainValidateIndex(gomock.Any(), abi.ChainEpoch(12), true).Return(nil, transient),
		full.EXPECT().ChainValidateIndex(gomock.Any(), abi.ChainEpoch(12), true).Return(valid(12), nil),
		full.EXPECT().ChainValidateIndex(gomock.Any(), abi.ChainEpoch(11), true).Return(nil, persistent),
		full.EXPECT().ChainValidateIndex(gomock.Any(), abi.ChainEpoch(11), true).Return(nil, persistent),
		// missing chain data halts the run without a retry
		full.EXPECT().ChainValidateIndex(gomock.Any(), abi.ChainEpoch(10), true).Return(nil, missing),
	)

	err := app.Run([]string{"lotus", "index", validateBackfillChainIndexCmd.Name, "--from", "13", "--to", "9"})
	require.ErrorContains(t, err, "halted at height 10")

	out := buf.String()
	require.Contains(t, out, "! Epoch 12; failure, retrying: transient failure")
	require.Contains(t, out, "✓ Epoch 12; succeeded on retry")
	require.NotContains(t, out, "✗ Epoch 12")
	require.Contains(t, out, "! Epoch 11; failure, retrying: persistent failure")
	require.Contains(t, out, "✗ Epoch 11; FAILED AGAIN ON RETRY: persistent failure")
	require.NotContains(t, out, "Epoch 10")
	require.Contains(t, out, "Total failed validations: 1")
	require.Contains(t, out, "Total validations that succeeded on retry: 1")
	require.Contains(t, out, "Total successful validations without backfilling: 2")
}

func TestValidateBackfillSummaryOmitsRetriesWhenNone(t *testing.T) {
	app, mockSrvcs, buf, done := newMockApp(t, IndexCmd)
	defer done()
	app.ErrWriter = io.Discard

	full := mocks.NewMockFullNode(gomock.NewController(t))
	mockSrvcs.EXPECT().FullNodeAPI().Return(full)
	mockSrvcs.EXPECT().Close().Return(nil)

	head := mock.MkBlock(nil, 0, 0)
	head.Height = 100
	full.EXPECT().ChainHead(gomock.Any()).Return(mock.TipSet(head), nil)
	full.EXPECT().ChainValidateIndex(gomock.Any(), gomock.Any(), true).
		DoAndReturn(func(_ any, epoch abi.ChainEpoch, _ bool) (*types.IndexValidation, error) {
			return &types.IndexValidation{Height: epoch}, nil
		}).Times(2)

	err := app.Run([]string{"lotus", "index", validateBackfillChainIndexCmd.Name, "--from", "11", "--to", "10"})
	require.NoError(t, err)

	out := buf.String()
	require.Contains(t, out, "Total failed validations: 0")
	require.NotContains(t, out, "retry")
}

func TestValidateBackfillQuietRetry(t *testing.T) {
	app, mockSrvcs, buf, done := newMockApp(t, IndexCmd)
	defer done()
	app.ErrWriter = io.Discard

	full := mocks.NewMockFullNode(gomock.NewController(t))
	mockSrvcs.EXPECT().FullNodeAPI().Return(full)
	mockSrvcs.EXPECT().Close().Return(nil)

	head := mock.MkBlock(nil, 0, 0)
	head.Height = 100
	full.EXPECT().ChainHead(gomock.Any()).Return(mock.TipSet(head), nil)
	gomock.InOrder(
		full.EXPECT().ChainValidateIndex(gomock.Any(), abi.ChainEpoch(10), true).Return(nil, errors.New("transient failure")),
		full.EXPECT().ChainValidateIndex(gomock.Any(), abi.ChainEpoch(10), true).Return(&types.IndexValidation{Height: 10}, nil),
	)

	err := app.Run([]string{"lotus", "index", validateBackfillChainIndexCmd.Name, "--from", "10", "--to", "10", "--quiet"})
	require.NoError(t, err)

	out := buf.String()
	require.Contains(t, out, "! Epoch 10; failure, retrying: transient failure")
	require.NotContains(t, out, "succeeded on retry")
}
