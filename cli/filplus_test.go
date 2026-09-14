package cli

import (
	"bytes"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/types"
)

func TestFilplusDeprecatedWrite(t *testing.T) {
	clientAddr, err := address.NewIDAddress(100)
	require.NoError(t, err)
	actorErr := errors.New("actor rejected message")

	for _, test := range []struct {
		name        string
		version     network.Version
		wantWarning bool
	}{
		{name: "before nv29", version: network.Version28, wantWarning: true},
		{name: "at nv29", version: network.Version29},
		{name: "after nv29", version: network.Version(30)},
	} {
		t.Run(test.name, func(t *testing.T) {
			app, mockAPI, stdout, done := NewMockAppWithFullAPI(t, FilplusCmd)
			defer done()
			stderr := new(bytes.Buffer)
			app.ErrWriter = stderr

			gomock.InOrder(
				mockAPI.EXPECT().StateNetworkVersion(gomock.Any(), types.EmptyTSK).Return(test.version, nil),
				mockAPI.EXPECT().StateLookupID(gomock.Any(), clientAddr, types.EmptyTSK).Return(clientAddr, nil),
				mockAPI.EXPECT().MpoolPushMessage(gomock.Any(), gomock.Any(), nil).Return(nil, actorErr),
			)

			err := app.Run([]string{"lotus", "filplus", "remove-expired-allocations", clientAddr.String(), "1"})
			require.ErrorIs(t, err, actorErr)
			require.Empty(t, stdout.String())
			if test.wantWarning {
				require.Contains(t, stderr.String(), "WARNING:")
				require.Contains(t, stderr.String(), "nv29")
				require.Contains(t, stderr.String(), "lotus-miner sectors upgrade-quality")
			} else {
				require.Empty(t, stderr.String())
			}
		})
	}
}

func TestFilplusDeprecatedWriteNetworkVersionError(t *testing.T) {
	app, mockAPI, stdout, done := NewMockAppWithFullAPI(t, FilplusCmd)
	defer done()
	stderr := new(bytes.Buffer)
	app.ErrWriter = stderr
	lookupErr := errors.New("network version unavailable")
	mockAPI.EXPECT().StateNetworkVersion(gomock.Any(), types.EmptyTSK).Return(network.Version(0), lookupErr)

	err := app.Run([]string{"lotus", "filplus", "remove-expired-allocations", "t0100", "1"})
	require.ErrorIs(t, err, lookupErr)
	require.Empty(t, stdout.String())
	require.Empty(t, stderr.String())
}
