package validation

import (
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/libp2p/go-libp2p-core/peer"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/chain-validation/pkg/chain"
	"github.com/filecoin-project/chain-validation/pkg/state"
	"github.com/filecoin-project/chain-validation/pkg/suites"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/address"
)

// A basic example validation test.
// At present this code is verbose and demonstrates the opportunity for helper methods.
func TestLotusExample(t *testing.T) {
	factory := NewFactories()
	drv := suites.NewStateDriver(t, factory.NewState())

	_, _, err := drv.State().SetSingletonActor(state.InitAddress, big.NewInt(0))
	require.NoError(t, err)

	alice := drv.NewAccountActor(2000)
	bob := drv.NewAccountActor(0)
	miner := drv.NewAccountActor(0) // Miner owner


	gasPrice := big.NewInt(1)
	gasLimit := state.GasUnit(1000)

	producer := chain.NewMessageProducer(factory.NewMessageFactory(drv.State()), gasLimit, gasPrice)
	msg, err := producer.Transfer(alice, bob, 0, 50)
	require.NoError(t, err)

	validator := chain.NewValidator(factory)
	exeCtx := chain.NewExecutionContext(1, miner)

	msgReceipt, err := validator.ApplyMessage(exeCtx, drv.State(), msg)
	require.NoError(t, err)
	require.NotNil(t, msgReceipt)

	expectedGasUsed := 126 // NB: should be derived from the size of the message + some other lotus VM bits
	assert.Equal(t, uint8(0), msgReceipt.ExitCode)
	assert.Empty(t, msgReceipt.ReturnValue)
	assert.Equal(t, state.GasUnit(expectedGasUsed), msgReceipt.GasUsed)

	drv.AssertBalance(alice, uint64(1950 - expectedGasUsed))
	drv.AssertBalance(bob, 50)
	// This should become non-zero after gas tracking and payments are integrated.
	drv.AssertBalance(miner, uint64(expectedGasUsed))

}

type testAddressWrapper struct {
	lotusAddr address.Address
	testAddr state.Address
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

func TestLotusCreateStorageMiner(t *testing.T) {
	factory := NewFactories()
	drv := suites.NewStateDriver(t, factory.NewState())

	gasPrice := big.NewInt(1)
	// gas prices will be inconsistent for a while, use a big value lotus team suggests using a large value here.
	gasLimit := state.GasUnit(1000000)
	TotalNetworkBalance := big.NewInt(1).Mul(big.NewInt(build.TotalFilecoin), big.NewInt(0).SetUint64(build.FilecoinPrecision))
	_, _, err := drv.State().SetSingletonActor(state.InitAddress, big.NewInt(0))
	require.NoError(t, err)
	_, _, err = drv.State().SetSingletonActor(state.NetworkAddress, TotalNetworkBalance)
	require.NoError(t, err)
	_, _, err = drv.State().SetSingletonActor(state.StoragePowerAddress, big.NewInt(0))
	require.NoError(t, err)


	// miner that mines in this test
	testMiner := newTestAddressWrapper(drv.NewAccountActor(0))
	// account that will own the miner
	minerOwner := newTestAddressWrapper(drv.NewAccountActor(20000000000))

	// address of the miner created
	maddr, err := state.NewIDAddress(102)
	require.NoError(t, err)
	minerAddr := newTestAddressWrapper(maddr)
	// sector size of the miner created
	sectorSize := big.NewInt(int64(build.SectorSizes[0]))
	// peerID of the miner created
	peerID, err := RequireIntPeerID(t, 1).MarshalBinary()
	require.NoError(t,err)
	// peerID of the miner after update
	peerID2, err := RequireIntPeerID(t,2).MarshalBinary()
	require.NoError(t,err)

	producer := chain.NewMessageProducer(factory.NewMessageFactory(drv.State()), gasLimit, gasPrice)
	validator := chain.NewValidator(factory)
	exeCtx := chain.NewExecutionContext(1, testMiner.testAddr)

	//
	// create a storage miner
	//
	msg, err := producer.StoragePowerCreateStorageMiner(minerOwner.testAddr, 0, minerOwner.testAddr, minerOwner.testAddr, sectorSize, peerID, chain.Value(2000000))
	require.NoError(t, err)
	msgReceipt, err := validator.ApplyMessage(exeCtx, drv.State(), msg)
	require.NoError(t, err)
	drv.AssertReceipt(msgReceipt, chain.MessageReceipt{
		ExitCode:    0,
		ReturnValue: []byte{0, 102},
		GasUsed:     0,
	})

	//
	// verify storage miners sector size
	//
	msg, err = producer.StorageMinerGetSectorSize(minerAddr.testAddr, minerOwner.testAddr, 1, chain.Value(2000000))
	require.NoError(t, err)
	msgReceipt, err = validator.ApplyMessage(exeCtx, drv.State(), msg)
	require.NoError(t, err)
	drv.AssertReceipt(msgReceipt, chain.MessageReceipt{
		ExitCode:    0,
		ReturnValue: sectorSize.Bytes(),
		GasUsed:     0,
	})


	//
	// verify storage miners owner
	//
	msg, err = producer.StorageMinerGetOwner(minerAddr.testAddr, minerOwner.testAddr, 2, chain.Value(2000000))
	require.NoError(t, err)
	msgReceipt, err = validator.ApplyMessage(exeCtx, drv.State(), msg)
	require.NoError(t, err)
	drv.AssertReceipt(msgReceipt, chain.MessageReceipt{
		ExitCode:    0,
		ReturnValue: minerOwner.lotusAddr.Bytes(),
		GasUsed:     0,
	})

	//
	// verify storage miners power
	//
	msg, err = producer.StorageMinerGetPower(minerAddr.testAddr, minerOwner.testAddr, 3, chain.Value(2000000))
	require.NoError(t, err)
	msgReceipt, err = validator.ApplyMessage(exeCtx, drv.State(), msg)
	require.NoError(t, err)
	drv.AssertReceipt(msgReceipt, chain.MessageReceipt{
		ExitCode:    0,
		ReturnValue: []byte{},
		GasUsed:     0,
	})


	//
	// verify storage miner worker address
	//
	msg, err = producer.StorageMinerGetWorkerAddr(minerAddr.testAddr, minerOwner.testAddr, 4, chain.Value(2000000))
	require.NoError(t, err)
	msgReceipt, err = validator.ApplyMessage(exeCtx, drv.State(), msg)
	require.NoError(t, err)
	drv.AssertReceipt(msgReceipt, chain.MessageReceipt{
		ExitCode:    0,
		ReturnValue: minerOwner.lotusAddr.Bytes(),
		GasUsed:     0,
	})

	//
	// verify storage miner peerID
	//
	msg, err = producer.StorageMinerGetPeerID(minerAddr.testAddr, minerOwner.testAddr, 5, chain.Value(2000000))
	require.NoError(t, err)
	msgReceipt, err = validator.ApplyMessage(exeCtx, drv.State(), msg)
	require.NoError(t, err)
	drv.AssertReceipt(msgReceipt, chain.MessageReceipt{
		ExitCode:    0,
		ReturnValue: peerID,
		GasUsed:     0,
	})

	//
	// update peerID
	//
	msg, err = producer.StorageMinerUpdatePeerID(minerAddr.testAddr, minerOwner.testAddr, 6, peerID2, chain.Value(2000000))
	require.NoError(t, err)
	msgReceipt, err = validator.ApplyMessage(exeCtx, drv.State(), msg)
	require.NoError(t, err)
	drv.AssertReceipt(msgReceipt, chain.MessageReceipt{
		ExitCode:    0,
		ReturnValue: []byte(nil),
		GasUsed:     0,
	})

	//
	// verify storage miner peerID
	//
	msg, err = producer.StorageMinerGetPeerID(minerAddr.testAddr, minerOwner.testAddr, 7, chain.Value(2000000))
	require.NoError(t, err)
	msgReceipt, err = validator.ApplyMessage(exeCtx, drv.State(), msg)
	require.NoError(t, err)
	drv.AssertReceipt(msgReceipt, chain.MessageReceipt{
		ExitCode:    0,
		ReturnValue: peerID2,
		GasUsed:     0,
	})

}

// RequireIntPeerID takes in an integer and creates a unique peer id for it.
func RequireIntPeerID(t *testing.T, i int64) peer.ID {
	buf := make([]byte, 16)
	n := binary.PutVarint(buf, i)
	h, err := mh.Sum(buf[:n], mh.ID, -1)
	require.NoError(t, err)
	pid, err := peer.IDFromBytes(h)
	require.NoError(t, err)
	return pid
}
