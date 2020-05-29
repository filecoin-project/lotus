package vm

import (
	"fmt"

	"github.com/filecoin-project/go-address"
	addr "github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	vmr "github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/ipfs/go-cid"
)

// Pricelist provides prices for operations in the VM.
//
// Note: this interface should be APPEND ONLY since last chain checkpoint
type Pricelist interface {
	// OnChainMessage returns the gas used for storing a message of a given size in the chain.
	OnChainMessage(msgSize int) int64
	// OnChainReturnValue returns the gas used for storing the response of a message in the chain.
	OnChainReturnValue(dataSize int) int64

	// OnMethodInvocation returns the gas used when invoking a method.
	OnMethodInvocation(value abi.TokenAmount, methodNum abi.MethodNum) int64

	// OnIpldGet returns the gas used for storing an object
	OnIpldGet(dataSize int) int64
	// OnIpldPut returns the gas used for storing an object
	OnIpldPut(dataSize int) int64

	// OnCreateActor returns the gas used for creating an actor
	OnCreateActor() int64
	// OnDeleteActor returns the gas used for deleting an actor
	OnDeleteActor() int64

	OnVerifySignature(sigType crypto.SigType, planTextSize int) (int64, error)
	OnHashing(dataSize int) int64
	OnComputeUnsealedSectorCid(proofType abi.RegisteredProof, pieces []abi.PieceInfo) int64
	OnVerifySeal(info abi.SealVerifyInfo) int64
	OnVerifyPost(info abi.WindowPoStVerifyInfo) int64
	OnVerifyConsensusFault() int64
}

var prices = map[abi.ChainEpoch]Pricelist{
	abi.ChainEpoch(0): &pricelistV0{
		onChainMessageBase:        0,
		onChainMessagePerByte:     2,
		onChainReturnValuePerByte: 8,
		sendBase:                  5,
		sendTransferFunds:         5,
		sendInvokeMethod:          10,
		ipldGetBase:               10,
		ipldGetPerByte:            1,
		ipldPutBase:               20,
		ipldPutPerByte:            2,
		createActorBase:           40, // IPLD put + 20
		createActorExtra:          500,
		deleteActor:               -500, // -createActorExtra
		// Dragons: this cost is not persistable, create a LinearCost{a,b} struct that has a `.Cost(x) -> ax + b`
		verifySignature: map[crypto.SigType]func(int64) int64{
			crypto.SigTypeBLS:       func(x int64) int64 { return 3*x + 2 },
			crypto.SigTypeSecp256k1: func(x int64) int64 { return 3*x + 2 },
		},
		hashingBase:                  5,
		hashingPerByte:               2,
		computeUnsealedSectorCidBase: 100,
		verifySealBase:               2000,
		verifyPostBase:               700,
		verifyConsensusFault:         10,
	},
}

// PricelistByEpoch finds the latest prices for the given epoch
func PricelistByEpoch(epoch abi.ChainEpoch) Pricelist {
	// since we are storing the prices as map or epoch to price
	// we need to get the price with the highest epoch that is lower or equal to the `epoch` arg
	bestEpoch := abi.ChainEpoch(0)
	bestPrice := prices[bestEpoch]
	for e, pl := range prices {
		// if `e` happened after `bestEpoch` and `e` is earlier or equal to the target `epoch`
		if e > bestEpoch && e <= epoch {
			bestEpoch = e
			bestPrice = pl
		}
	}
	if bestPrice == nil {
		panic(fmt.Sprintf("bad setup: no gas prices available for epoch %d", epoch))
	}
	return bestPrice
}

type pricedSyscalls struct {
	under     vmr.Syscalls
	pl        Pricelist
	chargeGas func(int64)
}

// Verifies that a signature is valid for an address and plaintext.
func (ps pricedSyscalls) VerifySignature(signature crypto.Signature, signer addr.Address, plaintext []byte) error {
	c, err := ps.pl.OnVerifySignature(signature.Type, len(plaintext))
	if err != nil {
		return err
	}
	ps.chargeGas(c)
	return ps.under.VerifySignature(signature, signer, plaintext)
}

// Hashes input data using blake2b with 256 bit output.
func (ps pricedSyscalls) HashBlake2b(data []byte) [32]byte {
	ps.chargeGas(ps.pl.OnHashing(len(data)))
	return ps.under.HashBlake2b(data)
}

// Computes an unsealed sector CID (CommD) from its constituent piece CIDs (CommPs) and sizes.
func (ps pricedSyscalls) ComputeUnsealedSectorCID(reg abi.RegisteredProof, pieces []abi.PieceInfo) (cid.Cid, error) {
	ps.chargeGas(ps.pl.OnComputeUnsealedSectorCid(reg, pieces))
	return ps.under.ComputeUnsealedSectorCID(reg, pieces)
}

// Verifies a sector seal proof.
func (ps pricedSyscalls) VerifySeal(vi abi.SealVerifyInfo) error {
	ps.chargeGas(ps.pl.OnVerifySeal(vi))
	return ps.under.VerifySeal(vi)
}

// Verifies a proof of spacetime.
func (ps pricedSyscalls) VerifyPoSt(vi abi.WindowPoStVerifyInfo) error {
	ps.chargeGas(ps.pl.OnVerifyPost(vi))
	return ps.under.VerifyPoSt(vi)
}

// Verifies that two block headers provide proof of a consensus fault:
// - both headers mined by the same actor
// - headers are different
// - first header is of the same or lower epoch as the second
// - at least one of the headers appears in the current chain at or after epoch `earliest`
// - the headers provide evidence of a fault (see the spec for the different fault types).
// The parameters are all serialized block headers. The third "extra" parameter is consulted only for
// the "parent grinding fault", in which case it must be the sibling of h1 (same parent tipset) and one of the
// blocks in the parent of h2 (i.e. h2's grandparent).
// Returns nil and an error if the headers don't prove a fault.
func (ps pricedSyscalls) VerifyConsensusFault(h1 []byte, h2 []byte, extra []byte) (*runtime.ConsensusFault, error) {
	ps.chargeGas(ps.pl.OnVerifyConsensusFault())
	return ps.under.VerifyConsensusFault(h1, h2, extra)
}

func (ps pricedSyscalls) BatchVerifySeals(inp map[address.Address][]abi.SealVerifyInfo) (map[address.Address][]bool, error) {
	ps.chargeGas(0) // TODO: this is only called by the cron actor. Should we even charge gas?
	return ps.under.BatchVerifySeals(inp)
}
