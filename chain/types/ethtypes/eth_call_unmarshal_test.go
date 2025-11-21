package ethtypes

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEthCall_Unmarshal_InputPreferredOverData(t *testing.T) {
	// Both input and data present; input should be preferred
	js := `{"from":"0x0000000000000000000000000000000000000000","to":"0x1111111111111111111111111111111111111111","data":"0xdeadbeef","input":"0xbeadfeed"}`
	var c EthCall
	require.NoError(t, json.Unmarshal([]byte(js), &c))
	exp, err := DecodeHexString("0xbeadfeed")
	require.NoError(t, err)
	require.Equal(t, EthBytes(exp), c.Data)
}

func TestEthCall_Unmarshal_DataFallback(t *testing.T) {
	js := `{"from":"0x0000000000000000000000000000000000000000","to":"0x1111111111111111111111111111111111111111","data":"0xdeadbeef"}`
	var c EthCall
	require.NoError(t, json.Unmarshal([]byte(js), &c))
	exp, err := DecodeHexString("0xdeadbeef")
	require.NoError(t, err)
	require.Equal(t, EthBytes(exp), c.Data)
}
