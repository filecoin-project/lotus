package mir

import (
	"github.com/ipfs/go-cid"
	u "github.com/ipfs/go-ipfs-util"

	addr "github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
)

type Validator struct {
	Addr addr.Address
	// FIXME: Consider using a multiaddr
	NetAddr string
}

func NewValidator(addr addr.Address, netAddr string) Validator {
	return Validator{
		addr, netAddr,
	}
}

func (v *Validator) ID() string {
	return v.Addr.String()
}

/*
func (v *Validator) Bytes() ([]byte, error) {
	var b buffer.Buffer
	if err := v.MarshalCBOR(&b); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// ValidatorsFromString parses comma-separated subnet:ID@OpaqueNetAddr validators string.
// OpaqueNetAddr can contain GRPC or Libp2p addresses.
//
// Examples of the validators:
// 	- /root:t1wpixt5mihkj75lfhrnaa6v56n27epvlgwparujy@/ip4/127.0.0.1/tcp/10000/p2p/12D3KooWJhKBXvytYgPCAaiRtiNLJNSFG5jreKDu2jiVpJetzvVJ
// 	- /root:t1wpixt5mihkj75lfhrnaa6v56n27epvlgwparujy@127.0.0.1:1000
func ValidatorsFromString(input string) ([]Validator, error) {
	var validators []Validator
	for _, idAddr := range SplitAndTrimEmpty(input, ",", " ") {
		parts := strings.Split(idAddr, "@")
		if len(parts) != 2 {
			return nil, fmt.Errorf("failed to parse validators string")
		}
		subnetAndID := parts[0]
		opaqueNetAddr := parts[1]

		subnetAndIDParts := strings.Split(subnetAndID, ":")
		if len(subnetAndIDParts) != 2 {
			return nil, fmt.Errorf("failed to parse subnet and ID")
		}
		subnetStr := subnetAndIDParts[0]
		ID := subnetAndIDParts[1]

		a, err := addr.NewFromString(ID)
		if err != nil {
			return nil, err
		}

		subnet, err := addr.SubnetIDFromString(subnetStr)
		if err != nil {
			return nil, err
		}

		v := Validator{
			Subnet:  subnet,
			Addr:    a,
			NetAddr: opaqueNetAddr,
		}
		validators = append(validators, v)
	}
	return validators, nil
}

// ValidatorsToString adds a validator subnet, address and network address into a string.
func ValidatorsToString(validators []Validator) string {
	var s string
	for _, v := range validators {
		s += fmt.Sprintf("%s:%s@%s,", v.Subnet.String(), v.Addr.String(), v.NetAddr)
	}
	return strings.TrimSuffix(s, ",")
}

func SplitAndTrimEmpty(s, sep, cutset string) []string {
	if s == "" {
		return []string{}
	}

	spl := strings.Split(s, sep)
	nonEmptyStrings := make([]string, 0, len(spl))

	for i := 0; i < len(spl); i++ {
		element := strings.Trim(spl[i], cutset)
		if element != "" {
			nonEmptyStrings = append(nonEmptyStrings, element)
		}
	}

	return nonEmptyStrings
}
*/

type ValidatorSet struct {
	Validators []Validator
}

func NewValidatorSet(vals []Validator) *ValidatorSet {
	return &ValidatorSet{Validators: vals}
}

func (set *ValidatorSet) Size() int {
	return len(set.Validators)
}

func (set *ValidatorSet) Equal(o *ValidatorSet) bool {
	if set == nil && o == nil {
		return true
	}
	if set == nil || o == nil {
		return true
	}
	if set.Size() != o.Size() {
		return false
	}
	for i, v := range set.Validators {
		if v != o.Validators[i] {
			return false
		}
	}
	return true
}

func (set *ValidatorSet) Hash() ([]byte, error) {
	var hs []byte
	for _, v := range set.Validators {
		b, err := v.Bytes()
		if err != nil {
			return nil, err
		}
		hs = append(hs, b...)
	}
	return cid.NewCidV0(u.Hash(hs)).Bytes(), nil
}

func (set *ValidatorSet) GetValidators() []Validator {
	return set.Validators
}

func (set *ValidatorSet) HasValidatorWithID(id string) bool {
	for _, v := range set.Validators {
		if v.ID() == id {
			return true
		}
	}
	return false
}

// BlockMiner returns a miner assigned deterministically using round-robin for an epoch to assign a reward
// according to the rules of original Filecoin/Eudico consensus.
func (set *ValidatorSet) BlockMiner(epoch abi.ChainEpoch) addr.Address {
	i := int(epoch) % set.Size()
	return set.Validators[i].Addr
}
