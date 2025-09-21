package ethtypes

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"golang.org/x/xerrors"
)

// maxListElements restricts the amount of RLP list elements we'll read.
// The ETH API only ever reads EIP-1559 transactions, which are bounded by
// 12 elements exactly, so we play it safe and set exactly that limit here.
const maxListElements = 12

func EncodeRLP(val interface{}) ([]byte, error) {
	return encodeRLP(val)
}

func encodeRLPListItems(list []interface{}) (result []byte, err error) {
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

func encodeLength(length int) (lenInBytes []byte, err error) {
	if length == 0 {
		return nil, fmt.Errorf("cannot encode length: length should be larger than 0")
	}

	buf := new(bytes.Buffer)
	err = binary.Write(buf, binary.BigEndian, int64(length))
	if err != nil {
		return nil, err
	}

	firstNonZeroIndex := len(buf.Bytes()) - 1
	for i, b := range buf.Bytes() {
		if b != 0 {
			firstNonZeroIndex = i
			break
		}
	}

	res := buf.Bytes()[firstNonZeroIndex:]
	return res, nil
}

func encodeRLP(val interface{}) ([]byte, error) {
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
	case []interface{}:
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
		return nil, fmt.Errorf("input data should either be a list or a byte array")
	}
}

func DecodeRLP(data []byte) (interface{}, error) {
	res, consumed, err := decodeRLP(data)
	if err != nil {
		return nil, err
	}
	if consumed != len(data) {
		return nil, xerrors.Errorf("invalid rlp data: length %d, consumed %d", len(data), consumed)
	}
	return res, nil
}

func decodeRLP(data []byte) (res interface{}, consumed int, err error) {
	if len(data) == 0 {
		return data, 0, xerrors.Errorf("invalid rlp data: data cannot be empty")
	}
	if data[0] >= 0xf8 {
		listLenInBytes := int(data[0]) - 0xf7
		listLen, err := decodeLength(data[1:], listLenInBytes)
		if err != nil {
			return nil, 0, err
		}
		if 1+listLenInBytes+listLen > len(data) {
			return nil, 0, xerrors.Errorf("invalid rlp data: out of bound while parsing list")
		}
		result, err := decodeListElems(data[1+listLenInBytes:], listLen)
		return result, 1 + listLenInBytes + listLen, err
	} else if data[0] >= 0xc0 {
		length := int(data[0]) - 0xc0
		result, err := decodeListElems(data[1:], length)
		return result, 1 + length, err
	} else if data[0] >= 0xb8 {
		strLenInBytes := int(data[0]) - 0xb7
		strLen, err := decodeLength(data[1:], strLenInBytes)
		if err != nil {
			return nil, 0, err
		}
		totalLen := 1 + strLenInBytes + strLen
		if totalLen > len(data) || totalLen < 0 {
			return nil, 0, xerrors.Errorf("invalid rlp data: out of bound while parsing string")
		}
		return data[1+strLenInBytes : totalLen], totalLen, nil
	} else if data[0] >= 0x80 {
		length := int(data[0]) - 0x80
		if 1+length > len(data) {
			return nil, 0, xerrors.Errorf("invalid rlp data: out of bound while parsing string")
		}
		return data[1 : 1+length], 1 + length, nil
	}
	return []byte{data[0]}, 1, nil
}

func decodeLength(data []byte, lenInBytes int) (length int, err error) {
	if lenInBytes > len(data) || lenInBytes > 8 {
		return 0, xerrors.Errorf("invalid rlp data: out of bound while parsing list length")
	}
	var decodedLength int64
	r := bytes.NewReader(append(make([]byte, 8-lenInBytes), data[:lenInBytes]...))
	if err := binary.Read(r, binary.BigEndian, &decodedLength); err != nil {
		return 0, xerrors.Errorf("invalid rlp data: cannot parse string length: %w", err)
	}
	if decodedLength < 0 {
		return 0, xerrors.Errorf("invalid rlp data: negative string length")
	}

	totalLength := lenInBytes + int(decodedLength)
	if totalLength < 0 || totalLength > len(data) {
		return 0, xerrors.Errorf("invalid rlp data: out of bound while parsing list")
	}
	return int(decodedLength), nil
}

func decodeListElems(data []byte, length int) (res []interface{}, err error) {
	totalConsumed := 0
	result := []interface{}{}

	for i := 0; totalConsumed < length && i < maxListElements; i++ {
		elem, consumed, err := decodeRLP(data[totalConsumed:])
		if err != nil {
			return nil, xerrors.Errorf("invalid rlp data: cannot decode list element: %w", err)
		}
		totalConsumed += consumed
		result = append(result, elem)
	}
	if totalConsumed != length {
		return nil, xerrors.Errorf("invalid rlp data: incorrect list length")
	}
	return result, nil
}
