package rpcenc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	blkfmt "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-jsonrpc"
)

type BlockHandler struct {
	calls atomic.Int64
}

func (h *BlockHandler) Echo(_ context.Context, b blkfmt.Block) (FlatBlock, error) {
	h.calls.Add(1)
	return FlatBlock{Cid: b.Cid(), RawData: b.RawData()}, nil
}

func TestBlockRPC(t *testing.T) {
	handler := &BlockHandler{}
	server := jsonrpc.NewServer(WithBlockfmtIfaceDecoder())
	server.Register("BlockHandler", handler)
	testServer := httptest.NewServer(server)
	defer testServer.Close()

	var client struct {
		Echo func(context.Context, blkfmt.Block) (FlatBlock, error)
	}
	closer, err := jsonrpc.NewMergeClient(context.Background(), testServer.URL, "BlockHandler", []interface{}{&client}, nil, WithBlockfmtIfaceEncoder())
	require.NoError(t, err)
	defer closer()

	for _, test := range []struct {
		name    string
		builder cid.Builder
		data    []byte
	}{
		{"cidv0", cid.V0Builder{}, []byte("block data")},
		{"dag-cbor", cid.V1Builder{Codec: cid.DagCBOR, MhType: multihash.BLAKE2B_MIN + 31}, []byte{0xa1, 0x61, 0x78, 0x18, 0x2a}},
		{"binary", cid.V1Builder{Codec: cid.Raw, MhType: multihash.SHA2_512}, []byte{0, 1, 0, 255}},
		{"truncated-hash", cid.V1Builder{Codec: cid.Raw, MhType: multihash.SHA2_256, MhLength: 16}, []byte("block data")},
		{"identity-hash", cid.V1Builder{Codec: cid.Raw, MhType: multihash.IDENTITY}, []byte("block data")},
		{"empty", cid.V1Builder{Codec: cid.Raw, MhType: multihash.SHA2_256}, []byte{}},
		{"nil", cid.V1Builder{Codec: cid.Raw, MhType: multihash.SHA2_256}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, err := test.builder.Sum(test.data)
			require.NoError(t, err)
			block, err := blkfmt.NewBlockWithCid(test.data, c)
			require.NoError(t, err)

			result, err := client.Echo(context.Background(), block)
			require.NoError(t, err)
			require.Equal(t, c, result.Cid)
			require.Equal(t, test.data, result.RawData)
		})
	}

	for _, test := range []struct {
		name  string
		block blkfmt.Block
	}{
		{"nil-block", nil},
		{"typed-nil-block", (*blkfmt.BasicBlock)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := handler.calls.Load()
			_, err := client.Echo(context.Background(), test.block)
			require.Error(t, err)
			require.Equal(t, calls, handler.calls.Load(), "nil blocks must not reach the handler")
		})
	}
}

func TestBlockRPCRejectsInvalidPayload(t *testing.T) {
	block := blkfmt.NewBlock([]byte("block data"))
	encodedCID, err := json.Marshal(block.Cid())
	require.NoError(t, err)
	unsupportedHash, err := multihash.Encode(make([]byte, 32), 0xdead)
	require.NoError(t, err)
	unsupportedCID, err := json.Marshal(cid.NewCidV1(cid.Raw, unsupportedHash))
	require.NoError(t, err)

	for _, test := range []struct {
		name    string
		payload string
	}{
		{"null", `null`},
		{"missing-cid", `{"RawData":"YmxvY2sgZGF0YQ=="}`},
		{"undefined-cid", `{"Cid":null,"RawData":"YmxvY2sgZGF0YQ=="}`},
		{"malformed-cid", `{"Cid":{"/":"not-a-cid"},"RawData":"YmxvY2sgZGF0YQ=="}`},
		{"invalid-base64", fmt.Sprintf(`{"Cid":%s,"RawData":"!"}`, encodedCID)},
		{"missing-data", fmt.Sprintf(`{"Cid":%s}`, encodedCID)},
		{"mismatched-data", fmt.Sprintf(`{"Cid":%s,"RawData":"b3RoZXIgZGF0YQ=="}`, encodedCID)},
		{"unsupported-hash", fmt.Sprintf(`{"Cid":%s,"RawData":"YmxvY2sgZGF0YQ=="}`, unsupportedCID)},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := &BlockHandler{}
			server := jsonrpc.NewServer(WithBlockfmtIfaceDecoder())
			server.Register("BlockHandler", handler)

			body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"BlockHandler.Echo","params":[%s]}`, test.payload)
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)

			var result struct {
				Error *jsonrpc.JSONRPCError `json:"error"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
			require.NotNil(t, result.Error)
			require.Contains(t, result.Error.Message, "custom decoder")
			require.Zero(t, handler.calls.Load(), "invalid blocks must not reach the handler")
		})
	}
}
