package address

import (
	"encoding/base32"

	"github.com/minio/blake2b-simd"
	errors "github.com/pkg/errors"
)

func init() {

	var err error

	TestAddress, err = NewActorAddress([]byte("satoshi"))
	if err != nil {
		panic(err)
	}

	TestAddress2, err = NewActorAddress([]byte("nakamoto"))
	if err != nil {
		panic(err)
	}

	NetworkAddress, err = NewActorAddress([]byte("filecoin"))
	if err != nil {
		panic(err)
	}

	StorageMarketAddress, err = NewActorAddress([]byte("storage"))
	if err != nil {
		panic(err)
	}

	PaymentBrokerAddress, err = NewActorAddress([]byte("payments"))
	if err != nil {
		panic(err)
	}
}

var (
	// TestAddress is an account with some initial funds in it.
	TestAddress Address
	// TestAddress2 is an account with some initial funds in it.
	TestAddress2 Address

	// NetworkAddress is the filecoin network.
	NetworkAddress Address
	// StorageMarketAddress is the hard-coded address of the filecoin storage market.
	StorageMarketAddress Address
	// PaymentBrokerAddress is the hard-coded address of the filecoin storage market.
	PaymentBrokerAddress Address
)

var (
	// ErrUnknownNetwork is returned when encountering an unknown network in an address.
	ErrUnknownNetwork = errors.New("unknown address network")

	// ErrUnknownProtocol is returned when encountering an unknown protocol in an address.
	ErrUnknownProtocol = errors.New("unknown address protocol")
	// ErrInvalidPayload is returned when encountering an invalid address payload.
	ErrInvalidPayload = errors.New("invalid address payload")
	// ErrInvalidLength is returned when encountering an address of invalid length.
	ErrInvalidLength = errors.New("invalid address length")
	// ErrInvalidChecksum is returned when encountering an invalid address checksum.
	ErrInvalidChecksum = errors.New("invalid address checksum")
)

// UndefAddressString is the string used to represent an empty address when encoded to a string.
var UndefAddressString = "<empty>"

// PayloadHashLength defines the hash length taken over addresses using the Actor and SECP256K1 protocols.
const PayloadHashLength = 20

// ChecksumHashLength defines the hash length used for calculating address checksums.
const ChecksumHashLength = 4

// MaxAddressStringLength is the max length of an address encoded as a string
// it include the network prefx, protocol, and bls publickey
const MaxAddressStringLength = 2 + 84

var payloadHashConfig = &blake2b.Config{Size: PayloadHashLength}
var checksumHashConfig = &blake2b.Config{Size: ChecksumHashLength}

const encodeStd = "abcdefghijklmnopqrstuvwxyz234567"

// AddressEncoding defines the base32 config used for address encoding and decoding.
var AddressEncoding = base32.NewEncoding(encodeStd)
