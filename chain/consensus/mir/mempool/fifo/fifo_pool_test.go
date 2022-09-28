package fifo

import (
	"testing"

	"github.com/ipfs/go-cid"
	u "github.com/ipfs/go-ipfs-util"
	"github.com/stretchr/testify/require"

	mirrequest "github.com/filecoin-project/mir/pkg/pb/requestpb"
)

func TestMirFIFOPool(t *testing.T) {
	p := New()

	c1 := cid.NewCidV0(u.Hash([]byte("req1")))
	c2 := cid.NewCidV0(u.Hash([]byte("req2")))

	inProgress := p.AddRequest(c1.String(), &mirrequest.Request{
		ClientId: "client1", Data: []byte{},
	})
	require.Equal(t, false, inProgress)

	inProgress = p.AddRequest(c1.String(), &mirrequest.Request{
		ClientId: "client1", Data: []byte{},
	})
	require.Equal(t, true, inProgress)

	inProgress = p.DeleteRequest(c1.String())
	require.Equal(t, true, inProgress)

	inProgress = p.DeleteRequest(c2.String())
	require.Equal(t, false, inProgress)

}
