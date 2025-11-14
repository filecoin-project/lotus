package ethtypes

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	abi "github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	typescrypto "github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
)

// EthAuthorization mirrors the 6-field tuple specified by EIP-7702:
//
//	[chain_id, address, nonce, y_parity, r, s]
//
// Reference: https://eips.ethereum.org/EIPS/eip-7702
type EthAuthorization struct {
	ChainID EthUint64  `json:"chainId"`
	Address EthAddress `json:"address"`
	Nonce   EthUint64  `json:"nonce"`
	YParity uint8      `json:"yParity"`
	R       EthBigInt  `json:"r"`
	S       EthBigInt  `json:"s"`
}

// DomainHash computes keccak256(0x05 || rlp([chain_id, address, nonce])) for this
// authorization tuple, as specified by EIP-7702. The chain ID used is the tuple's
// own chainId field (which may be 0 to indicate wildcard) and not the outer tx chain.
func (a EthAuthorization) DomainHash() ([32]byte, error) {
	return AuthorizationKeccak(uint64(a.ChainID), a.Address, uint64(a.Nonce))
}

// ---------- EIP-7702 Transaction (type 0x04) ----------
//
// rlp([chain_id, nonce, max_priority_fee_per_gas, max_fee_per_gas,
//
//	gas_limit, destination, value, data, access_list,
//	authorization_list, signature_y_parity, signature_r, signature_s])
//
// NOTE: access_list is currently required to be an empty list in Lotus'
// EIP-1559 parser; we follow that pattern here for now.
var _ EthTransaction = (*Eth7702TxArgs)(nil)

type Eth7702TxArgs struct {
	ChainID              int                `json:"chainId"`
	Nonce                int                `json:"nonce"`
	To                   *EthAddress        `json:"to"`
	Value                big.Int            `json:"value"`
	MaxFeePerGas         big.Int            `json:"maxFeePerGas"`
	MaxPriorityFeePerGas big.Int            `json:"maxPriorityFeePerGas"`
	GasLimit             int                `json:"gasLimit"`
	Input                []byte             `json:"input"`
	AuthorizationList    []EthAuthorization `json:"authorizationList"`

	// Outer signature (same style as EIP-1559): v(=y_parity), r, s
	V big.Int `json:"v"`
	R big.Int `json:"r"`
	S big.Int `json:"s"`
}

// ----- EthTransaction interface impls -----

func (tx *Eth7702TxArgs) Type() int { return EIP7702TxType }

func (tx *Eth7702TxArgs) ToRlpUnsignedMsg() ([]byte, error) {
	encoded, err := toRlpUnsignedMsg(tx)
	if err != nil {
		return nil, err
	}
	return append([]byte{EIP7702TxType}, encoded...), nil
}

func (tx *Eth7702TxArgs) ToRlpSignedMsg() ([]byte, error) {
	encoded, err := toRlpSignedMsg(tx, tx.V, tx.R, tx.S)
	if err != nil {
		return nil, err
	}
	return append([]byte{EIP7702TxType}, encoded...), nil
}

func (tx *Eth7702TxArgs) TxHash() (EthHash, error) {
	rlp, err := tx.ToRlpSignedMsg()
	if err != nil {
		return EmptyEthHash, err
	}
	return EthHashFromTxBytes(rlp), nil
}

func (tx *Eth7702TxArgs) Signature() (*typescrypto.Signature, error) {
	r := tx.R.Int.Bytes()
	s := tx.S.Int.Bytes()
	v := tx.V.Int.Bytes() // should be 0 or 1
	sig := append([]byte{}, padLeadingZeros(r, 32)...)
	sig = append(sig, padLeadingZeros(s, 32)...)
	if len(v) == 0 {
		sig = append(sig, 0)
	} else {
		sig = append(sig, v[0])
	}
	if len(sig) != 65 {
		return nil, xerrors.Errorf("signature is not 65 bytes")
	}
	return &typescrypto.Signature{
		Type: typescrypto.SigTypeDelegated,
		Data: sig,
	}, nil
}

func (tx *Eth7702TxArgs) ToVerifiableSignature(sig []byte) ([]byte, error) {
	// For typed tx with y_parity in v, the 65-byte signature is already in verifiable form.
	return sig, nil
}

func (tx *Eth7702TxArgs) Sender() (address.Address, error) { return sender(tx) }

// ToUnsignedFilecoinMessage delegates to ToUnsignedFilecoinMessageAtomic.
// Until the EthAccount/FVM support is fully landed, this path may return an error
// if the EthAccount.ApplyAndCall integration is not enabled.
func (tx *Eth7702TxArgs) ToUnsignedFilecoinMessage(from address.Address) (*types.Message, error) {
	return tx.ToUnsignedFilecoinMessageAtomic(from)
}

// ToUnsignedFilecoinMessageAtomic builds a Filecoin message that calls the
// EthAccount actor ApplyAndCall method with atomic apply+call semantics, encoding both
// the authorization list and the outer call (to/value/input) in params.
func (tx *Eth7702TxArgs) ToUnsignedFilecoinMessageAtomic(from address.Address) (*types.Message, error) {
	if tx.ChainID != buildconstants.Eip155ChainId {
		return nil, fmt.Errorf("invalid chain id: %d", tx.ChainID)
	}
	if !Eip7702FeatureEnabled {
		return nil, fmt.Errorf("EIP-7702 not yet wired to actors/FVM; parsed OK but cannot construct Filecoin message (enable actor integration to proceed)")
	}
	// CBOR encode [ [tuples...], [to(20b), value, input] ]
	var to EthAddress
	if tx.To != nil {
		to = *tx.To
	}
	params, err := CborEncodeEIP7702ApplyAndCall(tx.AuthorizationList, &to, tx.Value, tx.Input)
	if err != nil {
		return nil, xerrors.Errorf("failed to CBOR-encode apply+call params: %w", err)
	}
	if EthAccountApplyAndCallActorAddr == address.Undef {
		return nil, fmt.Errorf("EIP-7702 feature enabled but EthAccountApplyAndCallActorAddr is undefined; set ethtypes.EthAccountApplyAndCallActorAddr at init")
	}
	method := abi.MethodNum(MethodHash("ApplyAndCall"))
	return &types.Message{
		Version:    0,
		To:         EthAccountApplyAndCallActorAddr,
		From:       from,
		Nonce:      uint64(tx.Nonce),
		Value:      types.NewInt(0),
		GasLimit:   int64(tx.GasLimit),
		GasFeeCap:  tx.MaxFeePerGas,
		GasPremium: tx.MaxPriorityFeePerGas,
		Method:     method,
		Params:     params,
	}, nil
}

func (tx *Eth7702TxArgs) ToEthTx(smsg *types.SignedMessage) (EthTx, error) {
	from, err := EthAddressFromFilecoinAddress(smsg.Message.From)
	if err != nil {
		return EthTx{}, xerrors.Errorf("sender was not an eth account")
	}
	hash, err := tx.TxHash()
	if err != nil {
		return EthTx{}, err
	}
	gasFeeCap := EthBigInt(tx.MaxFeePerGas)
	gasPremium := EthBigInt(tx.MaxPriorityFeePerGas)
	ethTx := EthTx{
		ChainID:              EthUint64(buildconstants.Eip155ChainId),
		Type:                 EIP7702TxType,
		Nonce:                EthUint64(tx.Nonce),
		Hash:                 hash,
		To:                   tx.To,
		Value:                EthBigInt(tx.Value),
		Input:                tx.Input,
		Gas:                  EthUint64(tx.GasLimit),
		MaxFeePerGas:         &gasFeeCap,
		MaxPriorityFeePerGas: &gasPremium,
		From:                 from,
		R:                    EthBigInt(tx.R),
		S:                    EthBigInt(tx.S),
		V:                    EthBigInt(tx.V),
		AuthorizationList:    tx.AuthorizationList,
	}
	return ethTx, nil
}

func (tx *Eth7702TxArgs) InitialiseSignature(sig typescrypto.Signature) error {
	if sig.Type != typescrypto.SigTypeDelegated {
		return xerrors.Errorf("RecoverSignature only supports Delegated signature")
	}
	if len(sig.Data) != 65 {
		return xerrors.Errorf("signature should be 65 bytes long, but got %d bytes", len(sig.Data))
	}
	r_, err := parseBigInt(sig.Data[0:32])
	if err != nil {
		return xerrors.Errorf("cannot parse r into EthBigInt")
	}
	s_, err := parseBigInt(sig.Data[32:64])
	if err != nil {
		return xerrors.Errorf("cannot parse s into EthBigInt")
	}
	v_, err := parseBigInt([]byte{sig.Data[64]})
	if err != nil {
		return xerrors.Errorf("cannot parse v into EthBigInt")
	}
	tx.R = r_
	tx.S = s_
	tx.V = v_
	return nil
}

// ----- RLP packing / parsing -----

// Matches Eth1559 packer pattern, but inserts authorizationList before the signature fields.
func (tx *Eth7702TxArgs) packTxFields() ([]interface{}, error) {
	chainId, err := formatInt(tx.ChainID)
	if err != nil {
		return nil, err
	}
	nonce, err := formatInt(tx.Nonce)
	if err != nil {
		return nil, err
	}
	maxPrio, err := formatBigInt(tx.MaxPriorityFeePerGas)
	if err != nil {
		return nil, err
	}
	maxFee, err := formatBigInt(tx.MaxFeePerGas)
	if err != nil {
		return nil, err
	}
	gasLimit, err := formatInt(tx.GasLimit)
	if err != nil {
		return nil, err
	}
	value, err := formatBigInt(tx.Value)
	if err != nil {
		return nil, err
	}
	authList, err := packAuthorizationList(tx.AuthorizationList)
	if err != nil {
		return nil, err
	}
	res := []interface{}{
		chainId,
		nonce,
		maxPrio,
		maxFee,
		gasLimit,
		formatEthAddr(tx.To),
		value,
		tx.Input,
		[]interface{}{}, // accessList (not yet supported; keep empty)
		authList,        // authorizationList (list of 6-tuples)
	}
	return res, nil
}

func packAuthorizationList(list []EthAuthorization) ([]interface{}, error) {
	out := make([]interface{}, 0, len(list))
	for _, a := range list {
		ci, err := formatInt(int(a.ChainID))
		if err != nil {
			return nil, err
		}
		ni, err := formatInt(int(a.Nonce))
		if err != nil {
			return nil, err
		}
		ri, err := formatBigInt(big.Int(a.R))
		if err != nil {
			return nil, err
		}
		si, err := formatBigInt(big.Int(a.S))
		if err != nil {
			return nil, err
		}
		// y_parity is encoded as an integer (0/1)
		yp, err := formatInt(int(a.YParity))
		if err != nil {
			return nil, err
		}
		out = append(out, []interface{}{
			ci,
			a.Address[:],
			ni,
			yp,
			ri,
			si,
		})
	}
	return out, nil
}

// parseEip7702Tx decodes a type-0x04 (EIP-7702) transaction.
func parseEip7702Tx(data []byte) (*Eth7702TxArgs, error) {
	if data[0] != EIP7702TxType {
		return nil, xerrors.Errorf("not an EIP-7702 transaction: first byte is not %d", EIP7702TxType)
	}
	d, err := DecodeRLPWithLimit(data[1:], 13)
	if err != nil {
		return nil, err
	}
	decoded, ok := d.([]interface{})
	if !ok {
		return nil, xerrors.Errorf("not an EIP-7702 transaction: decoded data is not a list")
	}
	// 13 elements: 9 outer pre-accesslist fields, accessList, authorizationList, v, r, s
	if len(decoded) != 13 {
		return nil, xerrors.Errorf("not an EIP-7702 transaction: should have 13 elements in the rlp list")
	}

	chainId, err := parseInt(decoded[0])
	if err != nil {
		return nil, err
	}
	nonce, err := parseInt(decoded[1])
	if err != nil {
		return nil, err
	}
	maxPriorityFeePerGas, err := parseBigInt(decoded[2])
	if err != nil {
		return nil, err
	}
	maxFeePerGas, err := parseBigInt(decoded[3])
	if err != nil {
		return nil, err
	}
	gasLimit, err := parseInt(decoded[4])
	if err != nil {
		return nil, err
	}
	to, err := parseEthAddr(decoded[5])
	if err != nil {
		return nil, err
	}
	value, err := parseBigInt(decoded[6])
	if err != nil {
		return nil, err
	}
	input, err := parseBytes(decoded[7])
	if err != nil {
		return nil, err
	}
	// Access list: keep consistent with Lotus' EIP-1559 implementation for now (must be empty)
	if al, ok := decoded[8].([]interface{}); !ok || (ok && len(al) != 0) {
		return nil, xerrors.Errorf("access list should be an empty list")
	}

	// Authorization list
	authsIface, ok := decoded[9].([]interface{})
	if !ok {
		return nil, xerrors.Errorf("authorizationList is not a list")
	}
	if len(authsIface) == 0 {
		return nil, xerrors.Errorf("authorizationList must be non-empty")
	}
	auths := make([]EthAuthorization, 0, len(authsIface))
	// helper to enforce canonical unsigned int encoding (no leading zero if length > 1)
	parseUintCanonical := func(v interface{}) (uint64, error) {
		b, ok := v.([]byte)
		if !ok {
			return 0, xerrors.Errorf("not an RLP byte string")
		}
		// Reject non-canonical encodings: single 0x00 and any leading zero in multi-byte.
		if (len(b) == 1 && b[0] == 0x00) || (len(b) > 1 && b[0] == 0x00) {
			return 0, xerrors.Errorf("non-canonical integer encoding (leading zero)")
		}
		return parseUint64(v)
	}

	for i, it := range authsIface {
		t, ok := it.([]interface{})
		if !ok || len(t) != 6 {
			return nil, xerrors.Errorf("authorization[%d]: not a 6-field tuple", i)
		}
		ai, err := parseUintCanonical(t[0])
		if err != nil {
			return nil, xerrors.Errorf("authorization[%d]: bad chainId: %w", i, err)
		}
		addr, err := parseEthAddr(t[1])
		if err != nil {
			return nil, xerrors.Errorf("authorization[%d]: bad address: %w", i, err)
		}
		ni, err := parseUintCanonical(t[2])
		if err != nil {
			return nil, xerrors.Errorf("authorization[%d]: bad nonce: %w", i, err)
		}
		yp, err := parseUintCanonical(t[3])
		if err != nil {
			return nil, xerrors.Errorf("authorization[%d]: bad y_parity: %w", i, err)
		}
		if yp != 0 && yp != 1 {
			return nil, xerrors.Errorf("authorization[%d]: y_parity must be 0 or 1", i)
		}
		r, err := parseBigInt(t[4])
		if err != nil {
			return nil, xerrors.Errorf("authorization[%d]: bad r: %w", i, err)
		}
		s, err := parseBigInt(t[5])
		if err != nil {
			return nil, xerrors.Errorf("authorization[%d]: bad s: %w", i, err)
		}

		auths = append(auths, EthAuthorization{
			ChainID: EthUint64(ai),
			Address: func() EthAddress {
				if addr == nil {
					return EthAddress{}
				}
				return *addr
			}(),
			Nonce:   EthUint64(ni),
			YParity: uint8(yp),
			R:       EthBigInt(r),
			S:       EthBigInt(s),
		})
	}

	v, err := parseBigInt(decoded[10])
	if err != nil {
		return nil, err
	}
	r, err := parseBigInt(decoded[11])
	if err != nil {
		return nil, err
	}
	s, err := parseBigInt(decoded[12])
	if err != nil {
		return nil, err
	}
	// EIP-1559/2930 style v (y_parity) must be 0 or 1; same for 7702
	if !v.Equals(big.NewInt(0)) && !v.Equals(big.NewInt(1)) {
		return nil, xerrors.Errorf("EIP-7702 transactions only support 0 or 1 for v (y_parity)")
	}

	args := Eth7702TxArgs{
		ChainID:              chainId,
		Nonce:                nonce,
		To:                   to,
		Value:                value,
		Input:                input,
		MaxFeePerGas:         maxFeePerGas,
		MaxPriorityFeePerGas: maxPriorityFeePerGas,
		GasLimit:             gasLimit,
		AuthorizationList:    auths,
		V:                    v,
		R:                    r,
		S:                    s,
	}
	return &args, nil
}
