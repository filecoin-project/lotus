package genutils

import (
	"bytes"
	"context"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/rand"
	"github.com/filecoin-project/lotus/chain/types"
)

type MiningCheckAPI interface {
	StateGetRandomnessFromBeacon(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error)
	StateGetRandomnessFromTickets(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error)

	MinerGetBaseInfo(context.Context, address.Address, abi.ChainEpoch, types.TipSetKey) (*api.MiningBaseInfo, error)

	WalletSign(context.Context, address.Address, []byte) (*crypto.Signature, error)
}

func IsRoundWinner(ctx context.Context, round abi.ChainEpoch,
	miner address.Address, brand types.BeaconEntry, mbi *api.MiningBaseInfo, a MiningCheckAPI) (*types.ElectionProof, error) {

	buf := new(bytes.Buffer)
	if err := miner.MarshalCBOR(buf); err != nil {
		return nil, xerrors.Errorf("failed to cbor marshal address: %w", err)
	}

	electionRand, err := rand.DrawRandomnessFromBase(brand.Data, crypto.DomainSeparationTag_ElectionProofProduction, round, buf.Bytes())
	if err != nil {
		return nil, xerrors.Errorf("failed to draw randomness: %w", err)
	}

	vrfout, err := ComputeVRF(ctx, a.WalletSign, mbi.WorkerKey, electionRand)
	if err != nil {
		return nil, xerrors.Errorf("failed to compute VRF: %w", err)
	}

	ep := &types.ElectionProof{VRFProof: vrfout}
	j := ep.ComputeWinCount(mbi.MinerPower, mbi.NetworkPower)
	ep.WinCount = j
	if j < 1 {
		return nil, nil
	}

	return ep, nil
}

type SignFunc func(context.Context, address.Address, []byte) (*crypto.Signature, error)

func ComputeVRF(ctx context.Context, sign SignFunc, worker address.Address, sigInput []byte) ([]byte, error) {
	sig, err := sign(ctx, worker, sigInput)
	if err != nil {
		return nil, err
	}

	if sig.Type != crypto.SigTypeBLS {
		return nil, fmt.Errorf("miner worker address was not a BLS key")
	}

	return sig.Data, nil
}
