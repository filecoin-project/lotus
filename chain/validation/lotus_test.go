package validation

import (
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/libp2p/go-libp2p-core/peer"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/chain-validation/pkg/state"
	"github.com/filecoin-project/chain-validation/pkg/suites"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
)

func TestStorageMinerValidation(t *testing.T) {
	factory := NewFactories()
	suites.CreateStorageMinerAndUpdatePeerIDTest(t, factory)

}

func TestValueTransfer(t *testing.T) {
	factory := NewFactories()
	suites.AccountValueTransferTest(t, factory)
}

//
// some helpers tests to verify chain-validation can encode parameters to the CBOR lotus uses
//

func TestCBOREncodingCreateStorageMiner(t *testing.T) {
	oAddr, err := state.NewActorAddress([]byte("foobar"))
	require.NoError(t, err)
	ownerAddr := newTestAddressWrapper(oAddr)

	wAddr, err := state.NewActorAddress([]byte("barfoo"))
	require.NoError(t, err)
	workerAddr := newTestAddressWrapper(wAddr)

	lotusPeer := RequireIntPeerID(t, 1)
	bpid, err := lotusPeer.MarshalBinary()
	require.NoError(t, err)
	peerID := state.PeerID(bpid)

	lotusSectorSize := uint64(1)
	sectorSize := state.BytesAmount(big.NewInt(int64(lotusSectorSize)))

	lotusParams := actors.StorageMinerConstructorParams{
		Owner:      ownerAddr.lotusAddr,
		Worker:     workerAddr.lotusAddr,
		SectorSize: lotusSectorSize,
		PeerID:     lotusPeer,
	}
	lotusBytes, err := actors.SerializeParams(&lotusParams)
	require.NoError(t, err)

	specParams := []interface{}{ownerAddr.testAddr, workerAddr.testAddr, sectorSize, peerID}
	specBytes, err := state.EncodeValues(specParams...)
	require.NoError(t, err)

	assert.Equal(t, specBytes, lotusBytes)

}

func TestCBOREncodingPeerID(t *testing.T) {
	lotusPeer := RequireIntPeerID(t, 1)

	bpid, err := lotusPeer.MarshalBinary()
	require.NoError(t, err)
	peerID := state.PeerID(bpid)

	lotusParams := actors.UpdatePeerIDParams{
		PeerID: lotusPeer,
	}
	lotusBytes, err := actors.SerializeParams(&lotusParams)
	require.NoError(t, err)

	specParams := []interface{}{peerID}
	specBytes, err := state.EncodeValues(specParams...)
	require.NoError(t, err)

	assert.Equal(t, specBytes, lotusBytes)
}

func TestCBOREncodingUint64(t *testing.T) {
	lotusSectorSize := uint64(1)
	sectorSize := state.BytesAmount(big.NewInt(int64(lotusSectorSize)))

	lotusParams := actors.MultiSigChangeReqParams{Req: lotusSectorSize}
	lotusBytes, aerr := actors.SerializeParams(&lotusParams)
	require.NoError(t, aerr)

	specParams := []interface{}{sectorSize}
	specBytes, err := state.EncodeValues(specParams...)
	require.NoError(t, err)

	assert.Equal(t, specBytes, lotusBytes)
}

type testAddressWrapper struct {
	lotusAddr address.Address
	testAddr  state.Address
}

func newTestAddressWrapper(addr state.Address) *testAddressWrapper {
	la, err := address.NewFromBytes([]byte(addr))
	if err != nil {
		panic(err)
	}
	return &testAddressWrapper{
		lotusAddr: la,
		testAddr:  addr,
	}
}

// RequireIntPeerID takes in an integer and creates a unique peer id for it.
func RequireIntPeerID(t testing.TB, i int64) peer.ID {
	buf := make([]byte, 16)
	n := binary.PutVarint(buf, i)
	h, err := mh.Sum(buf[:n], mh.ID, -1)
	require.NoError(t, err)
	pid, err := peer.IDFromBytes(h)
	require.NoError(t, err)
	return pid
}
