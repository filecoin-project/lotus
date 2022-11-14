package types

import (
	"fmt"
	"io"

	"github.com/filecoin-project/go-state-types/exitcode"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

// This file contains custom CBOR serde logic to deal with the new versioned
// MessageReceipt resulting from the introduction of actor events (FIP-0049).

var lengthBufMessageReceipt = []byte{132}

// MarshalCBOR implements the standard marshalling logic, but omits the
// EventsRoot field when the version is 0.
func (mr *MessageReceipt) MarshalCBOR(w io.Writer) error {
	if mr == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}

	cw := cbg.NewCborWriter(w)

	if _, err := cw.Write(lengthBufMessageReceipt); err != nil {
		return err
	}

	// t.ExitCode (exitcode.ExitCode) (int64)
	if mr.ExitCode >= 0 {
		if err := cw.WriteMajorTypeHeader(cbg.MajUnsignedInt, uint64(mr.ExitCode)); err != nil {
			return err
		}
	} else {
		if err := cw.WriteMajorTypeHeader(cbg.MajNegativeInt, uint64(-mr.ExitCode-1)); err != nil {
			return err
		}
	}

	// t.Return ([]uint8) (slice)
	if len(mr.Return) > cbg.ByteArrayMaxLen {
		return xerrors.Errorf("Byte array in field t.Return was too long")
	}

	if err := cw.WriteMajorTypeHeader(cbg.MajByteString, uint64(len(mr.Return))); err != nil {
		return err
	}

	if _, err := cw.Write(mr.Return[:]); err != nil {
		return err
	}

	// t.GasUsed (int64) (int64)
	if mr.GasUsed >= 0 {
		if err := cw.WriteMajorTypeHeader(cbg.MajUnsignedInt, uint64(mr.GasUsed)); err != nil {
			return err
		}
	} else {
		if err := cw.WriteMajorTypeHeader(cbg.MajNegativeInt, uint64(-mr.GasUsed-1)); err != nil {
			return err
		}
	}

	//
	// Versioned fields.
	//

	if mr.version >= MessageReceiptVersion1 {
		// t.EventsRoot (cid.Cid) (struct)

		if mr.EventsRoot == nil {
			if _, err := cw.Write(cbg.CborNull); err != nil {
				return err
			}
		} else {
			if err := cbg.WriteCid(cw, *mr.EventsRoot); err != nil {
				return xerrors.Errorf("failed to write cid field t.EventsRoot: %w", err)
			}
		}
	}

	return nil
}

// UnmarshalCBOR implements the standard unmarshalling logic, but treats the
// EventsRoot field as optional, marking the MessageReceipt as a v0 if the field
// is entirely absent, and as v1 if it's present (even if NULL, which means that
// no events were emitted).
func (mr *MessageReceipt) UnmarshalCBOR(r io.Reader) (err error) {
	*mr = MessageReceipt{}

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

	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra != 4 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.ExitCode (exitcode.ExitCode) (int64)
	{
		maj, extra, err := cr.ReadHeader()
		var extraI int64
		if err != nil {
			return err
		}
		switch maj {
		case cbg.MajUnsignedInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 positive overflow")
			}
		case cbg.MajNegativeInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 negative oveflow")
			}
			extraI = -1 - extraI
		default:
			return fmt.Errorf("wrong type for int64 field: %d", maj)
		}

		mr.ExitCode = exitcode.ExitCode(extraI)
	}
	// t.Return ([]uint8) (slice)

	maj, extra, err = cr.ReadHeader()
	if err != nil {
		return err
	}

	if extra > cbg.ByteArrayMaxLen {
		return fmt.Errorf("t.Return: byte array too large (%d)", extra)
	}
	if maj != cbg.MajByteString {
		return fmt.Errorf("expected byte array")
	}

	if extra > 0 {
		mr.Return = make([]uint8, extra)
	}

	if _, err := io.ReadFull(cr, mr.Return[:]); err != nil {
		return err
	}
	// t.GasUsed (int64) (int64)
	{
		maj, extra, err := cr.ReadHeader()
		var extraI int64
		if err != nil {
			return err
		}
		switch maj {
		case cbg.MajUnsignedInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 positive overflow")
			}
		case cbg.MajNegativeInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 negative oveflow")
			}
			extraI = -1 - extraI
		default:
			return fmt.Errorf("wrong type for int64 field: %d", maj)
		}

		mr.GasUsed = int64(extraI)
	}

	//
	// Versioned fields.
	//

	// t.EventsRoot (cid.Cid) (struct)

	{
		b, err := cr.ReadByte()
		if err == io.EOF {
			// No more input to read, so this is an v0 message receipt.
			mr.version = MessageReceiptVersion0
			return nil
		}

		mr.version = MessageReceiptVersion1

		if err != nil {
			return err
		}
		if b != cbg.CborNull[0] {
			if err := cr.UnreadByte(); err != nil {
				return err
			}

			c, err := cbg.ReadCid(cr)
			if err != nil {
				return xerrors.Errorf("failed to read cid field t.EventsRoot: %w", err)
			}

			mr.EventsRoot = &c
		}

	}
	return nil
}
