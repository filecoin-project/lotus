package api

import (
	"bytes"
	"encoding/binary"

	"golang.org/x/xerrors"
)

func DecodeRLP(data []byte) (interface{}, error) {
	res, consumed, err := decodeRLP(data)
	if err != nil {
		return nil, err
	}
	if consumed != len(data) {
		return nil, xerrors.Errorf("cannot decode rlp: length %d, consumed %d", len(data), consumed)
	}
	return res, nil
}

func decodeRLP(data []byte) (res interface{}, consumed int, err error) {
	if data[0] >= 0xf8 {
		listLenInBytes := int(data[0]) - 0xf7
		listLen, err := decodeLength(data[1:], listLenInBytes)
		if err != nil {
			return nil, 0, err
		}
		if 1+listLenInBytes+listLen > len(data) {
			return nil, 0, xerrors.Errorf("invalid rlp data: out of bound while parsing list")
		}
		result, err := decodeListElems(data[1+listLenInBytes:], int(listLen))
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
		totalLen := 1 + strLenInBytes + int(strLen)
		if totalLen > len(data) {
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
	if lenInBytes+int(decodedLength) > len(data) {
		return 0, xerrors.Errorf("invalid rlp data: out of bound while parsing list")
	}
	return int(decodedLength), nil
}

func decodeListElems(data []byte, length int) (res []interface{}, err error) {
	totalConsumed := 0
	result := []interface{}{}

	for totalConsumed < length {
		elem, consumed, err := decodeRLP(data[totalConsumed:])
		if err != nil {
			return nil, xerrors.Errorf("invalid rlp data: cannot decode list element: %w", err)
		}
		totalConsumed += consumed
		result = append(result, elem)
	}
	if totalConsumed != length {
		return nil, xerrors.Errorf("invalid rlp data: incorrect list length", err)
	}
	return result, nil
}
