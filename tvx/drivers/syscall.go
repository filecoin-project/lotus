package drivers

import (
	"bytes"

	"github.com/filecoin-project/go-address"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/ipfs/go-cid"
	"github.com/minio/blake2b-simd"
)

var fakeVerifySignatureFunc = func(signature crypto.Signature, signer address.Address, plaintext []byte) error {
	return nil
}

var defaultHashBlake2bFunc = func(data []byte) [32]byte {
	return blake2b.Sum256(data)
}

var fakeComputerUnsealedSectorCIDFunc = func(proof abi.RegisteredSealProof, pieces []abi.PieceInfo) (cid.Cid, error) {
	// Fake CID computation by hashing the piece info (rather than the real computation over piece commitments).
	buf := bytes.Buffer{}
	for _, p := range pieces {
		err := p.MarshalCBOR(&buf)
		if err != nil {
			panic(err)
		}
	}
	token := blake2b.Sum256(buf.Bytes())
	return commcid.DataCommitmentV1ToCID(token[:])
}

var fakeVerifySealFunc = func(info abi.SealVerifyInfo) error {
	return nil
}

var fakeVerifyPoStFunc = func(info abi.WindowPoStVerifyInfo) error {
	return nil
}

var fakeBatchVerifySealfunc = func(inp map[address.Address][]abi.SealVerifyInfo) (map[address.Address][]bool, error) {
	out := make(map[address.Address][]bool)
	for a, svis := range inp {
		res := make([]bool, len(svis))
		for i := range res {
			res[i] = true
		}
		out[a] = res
	}
	return out, nil
}

var panicingVerifyConsensusFaultFunc = func(h1, h2, extra []byte) (*runtime.ConsensusFault, error) {
	panic("implement me")
}

type ChainValidationSysCalls struct {
	VerifySigFunc                func(signature crypto.Signature, signer address.Address, plaintext []byte) error
	HashBlake2bFunc              func(data []byte) [32]byte
	ComputeUnSealedSectorCIDFunc func(proof abi.RegisteredSealProof, pieces []abi.PieceInfo) (cid.Cid, error)
	VerifySealFunc               func(info abi.SealVerifyInfo) error
	VerifyPoStFunc               func(info abi.WindowPoStVerifyInfo) error
	VerifyConsensusFaultFunc     func(h1, h2, extra []byte) (*runtime.ConsensusFault, error)
	BatchVerifySealsFunc         func(map[address.Address][]abi.SealVerifyInfo) (map[address.Address][]bool, error)
}

func NewChainValidationSysCalls() *ChainValidationSysCalls {
	return &ChainValidationSysCalls{
		HashBlake2bFunc: defaultHashBlake2bFunc,

		VerifySigFunc:                fakeVerifySignatureFunc,
		ComputeUnSealedSectorCIDFunc: fakeComputerUnsealedSectorCIDFunc,
		VerifySealFunc:               fakeVerifySealFunc,
		VerifyPoStFunc:               fakeVerifyPoStFunc,

		VerifyConsensusFaultFunc: panicingVerifyConsensusFaultFunc,
		BatchVerifySealsFunc:     fakeBatchVerifySealfunc,
	}
}

func (c ChainValidationSysCalls) VerifySignature(signature crypto.Signature, signer address.Address, plaintext []byte) error {
	return c.VerifySigFunc(signature, signer, plaintext)
}

func (c ChainValidationSysCalls) HashBlake2b(data []byte) [32]byte {
	return c.HashBlake2bFunc(data)
}

func (c ChainValidationSysCalls) ComputeUnsealedSectorCID(proof abi.RegisteredSealProof, pieces []abi.PieceInfo) (cid.Cid, error) {
	return c.ComputeUnSealedSectorCIDFunc(proof, pieces)
}

func (c ChainValidationSysCalls) VerifySeal(info abi.SealVerifyInfo) error {
	return c.VerifySealFunc(info)
}

func (c ChainValidationSysCalls) VerifyPoSt(info abi.WindowPoStVerifyInfo) error {
	return c.VerifyPoStFunc(info)
}

func (c ChainValidationSysCalls) VerifyConsensusFault(h1, h2, extra []byte) (*runtime.ConsensusFault, error) {
	return c.VerifyConsensusFaultFunc(h1, h2, extra)
}

func (c ChainValidationSysCalls) BatchVerifySeals(inp map[address.Address][]abi.SealVerifyInfo) (map[address.Address][]bool, error) {
	return c.BatchVerifySealsFunc(inp)
}
