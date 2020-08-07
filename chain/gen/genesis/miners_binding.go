package genesis

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	cbor "github.com/ipfs/go-ipld-cbor"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"
)

func GetAccountMeta(account genesis.Actor) (*genesis.AccountMeta, error) {
	var meta genesis.AccountMeta
	meatb, err := account.Meta.MarshalJSON()
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(meatb, &meta)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

func setupBindMiners(ctx context.Context, vm *vm.VM, accounts []genesis.Actor) error {
	{
		actMap := make(map[address.Address]struct{})
		for _, account := range accounts {
			for _, m := range account.BindMiners {
				if _, ok := actMap[m.Address]; ok {
					return xerrors.Errorf("duplicate miner actor id %v", m.Address)
				}
			}
		}
	}
	var pid peer.ID
	{
		log.Warn("PeerID not specified, generating dummy")
		p, _, err := ic.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return err
		}

		pid, err = peer.IDFromPrivateKey(p)
		if err != nil {
			return err
		}
	}
	for _, account := range accounts {
		meta, err := GetAccountMeta(account)
		if err != nil {
			return xerrors.Errorf("failed to get account meta: %w", err)
		}
		for _, m := range account.BindMiners {

			constructorParams := &power.CreateMinerParams{
				Owner:         meta.Owner,
				Worker:        meta.Owner,
				Peer:          []byte(pid),
				SealProofType: m.SealProof,
			}

			err := vm.MutateState(ctx, builtin.InitActorAddr, func(cst cbor.IpldStore, st *init_.State) error {
				id, err := address.IDFromAddress(m.Address)
				if err != nil {
					return err
				}
				st.NextID = abi.ActorID(id)
				return nil
			})
			if err != nil {
				return xerrors.Errorf("mutating state: %w", err)
			}

			params := mustEnc(constructorParams)
			rval, err := doExecValue(ctx, vm, builtin.StoragePowerActorAddr, meta.Owner, types.NewInt(1), builtin.MethodsPower.CreateMiner, params)
			if err != nil {
				return xerrors.Errorf("failed to create genesis miner: %w", err)
			}

			var ma power.CreateMinerReturn
			if err := ma.UnmarshalCBOR(bytes.NewReader(rval)); err != nil {
				return xerrors.Errorf("unmarshaling CreateMinerReturn: %w", err)
			}

			if ma.IDAddress != m.Address {
				return xerrors.Errorf("miner assigned wrong address: %s != %s", ma.IDAddress, m.Address)
			}
		}
	}
	return nil
}
