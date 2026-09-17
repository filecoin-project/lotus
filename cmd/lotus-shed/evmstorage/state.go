// Package evmstorage enumerates and decodes the storage of EVM contracts
// on Filecoin. The EVM actor keeps contract storage in a KAMT<U256, U256>
// (bitWidth 5, maxArrayWidth 1, 32-byte big-endian keys); no on-chain
// method enumerates it, so the tools here walk the structure directly
// from its root CID.
package evmstorage

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/evm"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

// Contract is the storage-relevant view of an EVM actor at a tipset.
type Contract struct {
	EthAddress  ethtypes.EthAddress `json:"ethAddress"`
	IDAddress   address.Address     `json:"idAddress"`
	Nonce       uint64              `json:"nonce"`
	Alive       bool                `json:"alive"`
	Bytecode    cid.Cid             `json:"bytecode"`
	StorageRoot cid.Cid             `json:"storageRoot"`
	Transient   *Transient          `json:"transient,omitempty"`
}

// Transient is the FIP-0097 transient storage KAMT and the transaction
// lifespan it was written under (actors v16+; nil when never used or on
// older actor versions).
type Transient struct {
	Root   cid.Cid `json:"root"`
	Origin uint64  `json:"origin"`
	Nonce  uint64  `json:"nonce"`
}

// ResolveContract locates the EVM actor for addr (0x, f410, f0 or other
// filecoin address form) at ts and extracts the storage roots from its
// state.
func ResolveContract(ctx context.Context, api v1api.FullNode, store adt.Store, addr string, ts *types.TipSet) (*Contract, error) {
	filAddr, err := parseAddress(addr)
	if err != nil {
		return nil, err
	}
	idAddr, err := api.StateLookupID(ctx, filAddr, ts.Key())
	if err != nil {
		return nil, fmt.Errorf("resolving %s to an ID address: %w", filAddr, err)
	}
	ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(filAddr)
	if err != nil {
		// non-delegated input; the masked ID form is the best available
		ethAddr, err = ethtypes.EthAddressFromFilecoinAddress(idAddr)
		if err != nil {
			return nil, fmt.Errorf("deriving eth address for %s: %w", idAddr, err)
		}
	}

	act, err := api.StateGetActor(ctx, idAddr, ts.Key())
	if err != nil {
		return nil, fmt.Errorf("loading actor %s: %w", idAddr, err)
	}
	if err := ensureNetworkBundle(act.Code); err != nil {
		return nil, err
	}
	if !builtin.IsEvmActor(act.Code) {
		return nil, fmt.Errorf("actor %s is not an EVM contract (code %s)", idAddr, act.Code)
	}

	st, err := evm.Load(store, act)
	if err != nil {
		return nil, fmt.Errorf("loading EVM actor state: %w", err)
	}
	nonce, err := st.Nonce()
	if err != nil {
		return nil, err
	}
	alive, err := st.IsAlive()
	if err != nil {
		return nil, err
	}
	bytecode, err := st.GetBytecodeCID()
	if err != nil {
		return nil, err
	}

	c := &Contract{
		EthAddress: ethAddr,
		IDAddress:  idAddr,
		Nonce:      nonce,
		Alive:      alive,
		Bytecode:   bytecode,
	}
	if err := c.extractRoots(st.GetState()); err != nil {
		return nil, err
	}
	return c, nil
}

// extractRoots pulls the KAMT root CIDs out of the version-specific state
// struct. The evm.State interface does not expose them, and the struct
// type differs per actors version (TransientData exists only from v16),
// so reach in by field name.
func (c *Contract) extractRoots(state interface{}) error {
	sv := reflect.ValueOf(state)
	if sv.Kind() == reflect.Pointer {
		sv = sv.Elem()
	}
	cs := sv.FieldByName("ContractState")
	if !cs.IsValid() {
		return fmt.Errorf("EVM state %T has no ContractState field", state)
	}
	root, ok := cs.Interface().(cid.Cid)
	if !ok {
		return fmt.Errorf("EVM state %T ContractState is not a CID", state)
	}
	c.StorageRoot = root

	td := sv.FieldByName("TransientData")
	if !td.IsValid() || td.IsNil() {
		return nil
	}
	td = td.Elem()
	troot, ok := td.FieldByName("TransientDataState").Interface().(cid.Cid)
	if !ok {
		return fmt.Errorf("EVM state %T TransientDataState is not a CID", state)
	}
	lifespan := td.FieldByName("TransientDataLifespan")
	c.Transient = &Transient{
		Root:   troot,
		Origin: lifespan.FieldByName("Origin").Uint(),
		Nonce:  lifespan.FieldByName("Nonce").Uint(),
	}
	return nil
}

// ensureNetworkBundle switches the actor-code registry to the network the
// inspected actor belongs to. lotus-shed is built against one network's
// bundle but is frequently pointed at another; the actor loaders dispatch
// on code CIDs, so the matching manifest set must be active.
func ensureNetworkBundle(code cid.Cid) error {
	if _, _, ok := actors.GetActorMetaByCode(code); ok {
		return nil
	}
	for _, meta := range build.EmbeddedBuiltinActorsMetadata {
		for _, c := range meta.Actors {
			if c == code {
				return build.UseNetworkBundle(meta.Network)
			}
		}
	}
	return fmt.Errorf("actor code %s does not match any embedded builtin-actors bundle", code)
}

func parseAddress(s string) (address.Address, error) {
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		ea, err := ethtypes.ParseEthAddress(s)
		if err != nil {
			return address.Undef, fmt.Errorf("parsing eth address %q: %w", s, err)
		}
		return ea.ToFilecoinAddress()
	}
	a, err := address.NewFromString(s)
	if err != nil {
		return address.Undef, fmt.Errorf("parsing address %q: %w", s, err)
	}
	return a, nil
}
