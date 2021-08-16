package client

import (
	"fmt"
	"io"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/ipld/go-car/util"
	"github.com/multiformats/go-varint"
)

// —————————————————————————————————————————————————————————
//
//  This code is temporary, and should be deleted when
//  https://github.com/ipld/go-car/issues/196 is resolved.
//
// —————————————————————————————————————————————————————————

func init() {
	cbor.RegisterCborType(CarHeader{})
}

type CarHeader struct {
	Roots   []cid.Cid
	Version uint64
}

func readHeader(r io.Reader) (*CarHeader, error) {
	hb, err := ldRead(r, false)
	if err != nil {
		return nil, err
	}

	var ch CarHeader
	if err := cbor.DecodeInto(hb, &ch); err != nil {
		return nil, fmt.Errorf("invalid header: %v", err)
	}

	return &ch, nil
}

func writeHeader(h *CarHeader, w io.Writer) error {
	hb, err := cbor.DumpObject(h)
	if err != nil {
		return err
	}

	return util.LdWrite(w, hb)
}

func ldRead(r io.Reader, zeroLenAsEOF bool) ([]byte, error) {
	l, err := varint.ReadUvarint(toByteReader(r))
	if err != nil {
		// If the length of bytes read is non-zero when the error is EOF then signal an unclean EOF.
		if l > 0 && err == io.EOF {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, err
	} else if l == 0 && zeroLenAsEOF {
		return nil, io.EOF
	}

	buf := make([]byte, l)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	return buf, nil
}

type readerPlusByte struct {
	io.Reader
}

func (rb readerPlusByte) ReadByte() (byte, error) {
	return readByte(rb)
}

func readByte(r io.Reader) (byte, error) {
	var p [1]byte
	_, err := io.ReadFull(r, p[:])
	return p[0], err
}

func toByteReader(r io.Reader) io.ByteReader {
	if br, ok := r.(io.ByteReader); ok {
		return br
	}
	return &readerPlusByte{r}
}
