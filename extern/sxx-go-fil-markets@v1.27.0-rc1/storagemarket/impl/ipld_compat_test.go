package storageimpl_test

// TODO(rvagg): this is a transitional package to test compatibility between
// cbor-gen and bindnode - it can be removed if/when cbor-gen is also removed

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/ipld/go-ipld-prime/schema"
	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
)

func TestIpldCompat_StorageDataTransferVoucher(t *testing.T) {
	acid, err := cid.Decode("bafy2bzaceashdsqgbnisdg76gdhupvkpop4br5rs3veuy4whuxagnoco6px6e")
	assert.Nil(t, err)

	for _, sdtv := range []requestvalidation.StorageDataTransferVoucher{
		{Proposal: acid},
		// {}, - we can't test this because cbor-gen generates an invalid byte repr for empty CID
	} {
		t.Run(fmt.Sprintf("with Proposal: %s", sdtv.Proposal), func(t *testing.T) {
			// encode the StorageDataTransferVoucher with cbor-gen to bytes
			var originalBuf bytes.Buffer
			sdtv.MarshalCBOR(&originalBuf)
			originalBytes := originalBuf.Bytes()

			// decode the bytes to StorageDataTransferVoucher with bindnode
			nb := basicnode.Prototype.Any.NewBuilder()
			dagcbor.Decode(nb, &originalBuf)
			node := nb.Build()
			sdtvBindnodeIface, err := requestvalidation.BindnodeRegistry.TypeFromNode(node, &requestvalidation.StorageDataTransferVoucher{})
			assert.Nil(t, err)
			sdtvBindnode, ok := sdtvBindnodeIface.(*requestvalidation.StorageDataTransferVoucher)
			assert.True(t, ok)

			// compare objects
			assert.Equal(t, sdtv.Proposal, sdtvBindnode.Proposal)

			// encode the new StorageDataTransferVoucher with bindnode to bytes
			node = requestvalidation.BindnodeRegistry.TypeToNode(sdtvBindnode)
			var bindnodeBuf bytes.Buffer
			dagcbor.Encode(node.(schema.TypedNode).Representation(), &bindnodeBuf)
			bindnodeBytes := bindnodeBuf.Bytes()

			// compare bytes
			assert.Equal(t, originalBytes, bindnodeBytes)

			// decode the new bytes to StorageDataTransferVoucher with cbor-gen
			var roundtripSdtv requestvalidation.StorageDataTransferVoucher
			err = roundtripSdtv.UnmarshalCBOR(&bindnodeBuf)
			assert.Nil(t, err)

			// compare objects
			assert.Equal(t, sdtv.Proposal, roundtripSdtv.Proposal)
		})
	}
}
