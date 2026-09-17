package main

import (
	"bytes"
	"encoding/hex"
	"flag"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api/mocks"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

func TestMessageFromStringEthereumHash(t *testing.T) {
	const hashHex = "a00bb0b8ff8186f5aa957771a75923084864cfd785b62d1d42a5f7f424f8ce95"

	txHash, err := ethtypes.ParseEthHash(hashHex)
	require.NoError(t, err)
	messageCID := cid.MustParse("bafy2bzacecqaxmfy76ayn5nksv3xdj2zemeeqzgp26c3mli5iks7p5be7dhjk")
	from, err := address.NewIDAddress(100)
	require.NoError(t, err)
	to, err := address.NewIDAddress(101)
	require.NoError(t, err)
	message := types.Message{
		From: from,
		To:   to,
	}
	var encoded bytes.Buffer
	require.NoError(t, message.MarshalCBOR(&encoded))

	for _, input := range []string{hashHex, "0x" + hashHex} {
		t.Run(input[:2], func(t *testing.T) {
			ctrl := gomock.NewController(t)
			fullNode := mocks.NewMockFullNode(ctrl)
			fullNode.EXPECT().EthGetMessageCidByTransactionHash(gomock.Any(), &txHash).Return(&messageCID, nil)
			fullNode.EXPECT().ChainReadObj(gomock.Any(), messageCID).Return(encoded.Bytes(), nil)

			app := cli.NewApp()
			app.Metadata = map[string]interface{}{"testnode-full": fullNode}
			ctx := cli.NewContext(app, flag.NewFlagSet(t.Name(), flag.ContinueOnError), nil)

			got, err := messageFromString(ctx, input)
			require.NoError(t, err)
			require.Equal(t, message.Cid(), got.Cid())
		})
	}
}

func TestMessageFromStringRawMessageHex(t *testing.T) {
	from, err := address.NewIDAddress(100)
	require.NoError(t, err)
	to, err := address.NewIDAddress(101)
	require.NoError(t, err)
	message := types.Message{
		From:   from,
		To:     to,
		Params: make([]byte, 17),
	}
	var encoded bytes.Buffer
	require.NoError(t, message.MarshalCBOR(&encoded))
	require.Len(t, encoded.Bytes(), ethtypes.EthHashLength)

	app := cli.NewApp()
	ctx := cli.NewContext(app, flag.NewFlagSet(t.Name(), flag.ContinueOnError), nil)

	got, err := messageFromString(ctx, hex.EncodeToString(encoded.Bytes()))
	require.NoError(t, err)
	require.Equal(t, message.Cid(), got.Cid())
}
