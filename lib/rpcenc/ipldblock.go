package rpcenc

import (
	"context"
	"encoding/json"
	"reflect"

	blkfmt "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"
)

// Among all the methods in api.FullNode, there is only one with an interface-argument:
// the go-block-format.Block in ChainPutObj
//
// interfaces are not natively supported by the jsonrpc
//
// These are the custom client/server handlers to make ChainPutObj work

// FlatBlock is a representation of blkfmt.Bloc that would travel without a problem over jsonrpc
type FlatBlock struct {
	Cid     cid.Cid
	RawData []byte
}

func WithBlockfmtIfaceEncoder() jsonrpc.Option {
	return jsonrpc.WithParamEncoder(new(blkfmt.Block), func(v reflect.Value) (reflect.Value, error) {
		b := v.Interface().(blkfmt.Block)
		return reflect.ValueOf(FlatBlock{
			Cid:     b.Cid(),
			RawData: b.RawData(),
		}), nil
	})
}

func WithBlockfmtIfaceDecoder() jsonrpc.ServerOption {
	return jsonrpc.WithParamDecoder(new(blkfmt.Block), func(_ context.Context, j []byte) (reflect.Value, error) {
		var fb FlatBlock
		if err := json.Unmarshal(j, &fb); err != nil {
			return reflect.Value{}, xerrors.Errorf("decoding blkfmt.Block param: %w", err)
		}

		if fb.Cid == cid.Undef {
			return reflect.Value{}, xerrors.Errorf("invalid Block payload: %s", j)
		}

		blk, err := blkfmt.NewBlockWithCid(fb.RawData, fb.Cid)
		if err != nil {
			return reflect.Value{}, xerrors.Errorf("constructing block: %w", err)
		}

		// always verify - who knows what comes down the RPC...
		if rehashCid, err := fb.Cid.Prefix().Sum(fb.RawData); err != nil {
			return reflect.Value{}, xerrors.Errorf("rehash failed: %w", err)
		} else if !rehashCid.Equals(fb.Cid) {
			return reflect.Value{}, xerrors.Errorf("supplied cid %s does not match rehash %s of the supplied data hash failed", fb.Cid, rehashCid)
		}

		return reflect.ValueOf(blk), nil
	})
}
