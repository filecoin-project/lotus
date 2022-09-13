package jsonfield

import (
	"encoding/json"
	"fmt"
	"io"

	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

// JSONBytes is a wrapper for types which are marshalled into a base64 encoded json-string
type JSONBytes[T any] struct {
	Data T
}

func NewJsonBytes[T any](data T) *JSONBytes[T] {
	return &JSONBytes[T]{
		Data: data,
	}
}

func JsonBytesFrom[T any](data []byte) (out JSONBytes[T], err error) {
	err = out.UnmarshalJSON(data)
	return
}

func (j *JSONBytes[T]) MarshalJSON() ([]byte, error) {
	jb, err := json.Marshal(&j.Data)
	if err != nil {
		return nil, err
	}

	return json.Marshal(jb)
}

func (j *JSONBytes[T]) UnmarshalJSON(data []byte) error {
	var js []byte
	if err := json.Unmarshal(data, &js); err != nil {
		return err
	}

	return json.Unmarshal(js, &j.Data)
}

func (j *JSONBytes[T]) MarshalCBOR(w io.Writer) error {
	if j == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}

	jb, err := json.Marshal(&j.Data)
	if err != nil {
		return err
	}

	cw := cbg.NewCborWriter(w)

	if len(jb) > cbg.ByteArrayMaxLen {
		return xerrors.Errorf("byte array in JSONBytes field was too long")
	}

	if err := cw.WriteMajorTypeHeader(cbg.MajByteString, uint64(len(jb))); err != nil {
		return err
	}

	if _, err := cw.Write(jb[:]); err != nil {
		return err
	}

	return nil
}

func (j *JSONBytes[T]) UnmarshalCBOR(r io.Reader) (err error) {
	cr := cbg.NewCborReader(r)

	maj, extra, err := cr.ReadHeader()
	if err != nil {
		return err
	}
	defer func() {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
	}()

	if extra > cbg.ByteArrayMaxLen {
		return fmt.Errorf("JSONBytes byte array too large (%d)", extra)
	}
	if maj != cbg.MajByteString {
		return fmt.Errorf("expected byte array")
	}

	var jb []byte

	if extra > 0 {
		jb = make([]byte, extra)
	}

	if _, err := io.ReadFull(cr, jb[:]); err != nil {
		return err
	}

	jj, err := JsonBytesFrom[T](jb)
	if err != nil {
		return err
	}

	*j = jj
	return nil
}
