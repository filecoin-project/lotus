package ethtypes

import (
	"encoding/binary"
	"errors"
	"fmt"

	"golang.org/x/xerrors"
)

const (
	// maxListElements limits the number of elements decoded per RLP list.
	// Of the transaction formats we decode, EIP-1559 has the most fields
	// at 12 (legacy has 9). This is set to match that upper bound.
	maxListElements = 12

	// maxListDepth limits how deeply RLP lists may be nested. Ethereum
	// transactions nest at most 3 levels deep (tx list > access list >
	// [address, storageKeys] tuple > keys list). 32 provides generous
	// headroom for any future transaction format.
	maxListDepth = 32

	// maxInputLen is the largest RLP blob we will attempt to decode.
	// Contract-deploy transactions on Filecoin can carry up to 1 MiB of
	// init-code; 2 MiB gives comfortable headroom after RLP framing.
	maxInputLen = 2 << 20 // 2 MiB
)

var (
	errRLPEmptyInput          = errors.New("invalid rlp data: data is empty")
	errRLPNonCanonicalList    = errors.New("invalid rlp data: non-canonical list length encoding")
	errRLPNonCanonicalString  = errors.New("invalid rlp data: non-canonical string length encoding")
	errRLPNonCanonicalByte    = errors.New("invalid rlp data: non-canonical single byte encoding")
	errRLPNonCanonicalLeading = errors.New("invalid rlp data: non-canonical length encoding with leading zeros")
	errRLPListBounds          = errors.New("invalid rlp data: out of bound while parsing list")
	errRLPStringBounds        = errors.New("invalid rlp data: out of bound while parsing string")
	errRLPLengthBounds        = errors.New("invalid rlp data: out of bound while parsing length")
	errRLPIncorrectListLen    = errors.New("invalid rlp data: incorrect list length")
	errRLPEncodeZeroLength    = errors.New("cannot encode length: length should be larger than 0")
	errRLPEncodeInvalidType   = errors.New("input data should either be a list or a byte array")
)

func EncodeRLP(val any) ([]byte, error) {
	return encodeRLP(val)
}

func encodeRLPListItems(list []any) (result []byte, err error) {
	res := []byte{}
	for _, elem := range list {
		encoded, err := encodeRLP(elem)
		if err != nil {
			return nil, err
		}
		res = append(res, encoded...)
	}
	return res, nil
}

// encodeLength returns the minimal big-endian representation of length.
func encodeLength(length int) ([]byte, error) {
	if length == 0 {
		return nil, errRLPEncodeZeroLength
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(length))
	for i, b := range buf {
		if b != 0 {
			return append([]byte{}, buf[i:]...), nil
		}
	}
	return nil, errRLPEncodeZeroLength
}

func encodeRLP(val any) ([]byte, error) {
	switch data := val.(type) {
	case []byte:
		if len(data) == 1 && data[0] <= 0x7f {
			return data, nil
		} else if len(data) <= 55 {
			prefix := byte(0x80 + len(data))
			return append([]byte{prefix}, data...), nil
		}
		lenInBytes, err := encodeLength(len(data))
		if err != nil {
			return nil, err
		}
		prefix := byte(0xb7 + len(lenInBytes))
		return append(
			[]byte{prefix},
			append(lenInBytes, data...)...,
		), nil
	case []any:
		encodedList, err := encodeRLPListItems(data)
		if err != nil {
			return nil, err
		}
		if len(encodedList) <= 55 {
			prefix := byte(0xc0 + len(encodedList))
			return append(
				[]byte{prefix},
				encodedList...,
			), nil
		}
		lenInBytes, err := encodeLength(len(encodedList))
		if err != nil {
			return nil, err
		}
		prefix := byte(0xf7 + len(lenInBytes))
		return append(
			[]byte{prefix},
			append(lenInBytes, encodedList...)...,
		), nil
	default:
		return nil, errRLPEncodeInvalidType
	}
}

func DecodeRLP(data []byte) (any, error) {
	if len(data) == 0 {
		return nil, errRLPEmptyInput
	}
	if len(data) > maxInputLen {
		return nil, fmt.Errorf("invalid rlp data: input length %d exceeds limit of %d", len(data), maxInputLen)
	}
	res, consumed, err := decodeRLP(data, 0)
	if err != nil {
		return nil, err
	}
	if consumed != len(data) {
		return nil, fmt.Errorf("invalid rlp data: length %d, consumed %d", len(data), consumed)
	}
	return res, nil
}

func decodeRLP(data []byte, depth int) (res any, consumed int, err error) {
	if len(data) == 0 {
		return nil, 0, errRLPEmptyInput
	}

	b := data[0]

	switch {
	case b >= 0xf8: // long list (> 55 bytes content)
		if depth > maxListDepth {
			return nil, 0, fmt.Errorf("invalid rlp data: list nesting depth exceeds limit of %d", maxListDepth)
		}
		listLenInBytes := int(b) - 0xf7
		listLen, err := decodeLength(data[1:], listLenInBytes)
		if err != nil {
			return nil, 0, err
		}
		if listLen < 56 {
			return nil, 0, errRLPNonCanonicalList
		}
		if 1+listLenInBytes+listLen > len(data) {
			return nil, 0, errRLPListBounds
		}
		result, err := decodeListElems(data[1+listLenInBytes:], listLen, depth+1)
		return result, 1 + listLenInBytes + listLen, err

	case b >= 0xc0: // short list (0-55 bytes content)
		if depth > maxListDepth {
			return nil, 0, fmt.Errorf("invalid rlp data: list nesting depth exceeds limit of %d", maxListDepth)
		}
		length := int(b) - 0xc0
		if 1+length > len(data) {
			return nil, 0, errRLPListBounds
		}
		result, err := decodeListElems(data[1:], length, depth+1)
		return result, 1 + length, err

	case b >= 0xb8: // long string (> 55 bytes)
		strLenInBytes := int(b) - 0xb7
		strLen, err := decodeLength(data[1:], strLenInBytes)
		if err != nil {
			return nil, 0, err
		}
		if strLen < 56 {
			return nil, 0, errRLPNonCanonicalString
		}
		totalLen := 1 + strLenInBytes + strLen
		if totalLen > len(data) || totalLen < 0 {
			return nil, 0, errRLPStringBounds
		}
		return data[1+strLenInBytes : totalLen], totalLen, nil

	case b >= 0x80: // short string (0-55 bytes)
		length := int(b) - 0x80
		if 1+length > len(data) {
			return nil, 0, errRLPStringBounds
		}
		if length == 1 && data[1] < 0x80 {
			return nil, 0, errRLPNonCanonicalByte
		}
		return data[1 : 1+length], 1 + length, nil

	default: // single byte [0x00, 0x7f]
		return []byte{b}, 1, nil
	}
}

// decodeLength reads a big-endian length from the first lenInBytes of data.
func decodeLength(data []byte, lenInBytes int) (int, error) {
	if lenInBytes > len(data) || lenInBytes > 8 {
		return 0, errRLPLengthBounds
	}
	if lenInBytes > 0 && data[0] == 0 {
		return 0, errRLPNonCanonicalLeading
	}
	var buf [8]byte
	copy(buf[8-lenInBytes:], data[:lenInBytes])
	length := binary.BigEndian.Uint64(buf[:])
	if length > uint64(len(data)-lenInBytes) {
		return 0, errRLPLengthBounds
	}
	return int(length), nil
}

func decodeListElems(data []byte, length int, depth int) (res []any, err error) {
	totalConsumed := 0
	result := []any{}

	for i := 0; totalConsumed < length && i < maxListElements; i++ {
		elem, consumed, err := decodeRLP(data[totalConsumed:], depth)
		if err != nil {
			return nil, xerrors.Errorf("invalid rlp data: cannot decode list element: %w", err)
		}
		totalConsumed += consumed
		result = append(result, elem)
	}
	if totalConsumed != length {
		return nil, errRLPIncorrectListLen
	}
	return result, nil
}
