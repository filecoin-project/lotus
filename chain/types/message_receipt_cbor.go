package types

import (
	"fmt"
	"io"

	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/exitcode"
)

// This file contains custom CBOR serde logic to deal with the new versioned
// MessageReceipt resulting from the introduction of actor events (FIP-0049).

type messageReceiptV0 struct{ *MessageReceipt }

type messageReceiptV1 struct{ *MessageReceipt }

func (mr *MessageReceipt) MarshalCBOR(w io.Writer) error {
	if mr == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}

	var m cbor.Marshaler
	switch mr.version {
	case MessageReceiptV0:
		m = &messageReceiptV0{mr}
	case MessageReceiptV1:
		m = &messageReceiptV1{mr}
	default:
		return xerrors.Errorf("invalid message receipt version: %d", mr.version)
	}

	return m.MarshalCBOR(w)
}

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

	var u cbor.Unmarshaler
	switch extra {
	case 3:
		mr.version = MessageReceiptV0
		u = &messageReceiptV0{mr}
	case 4:
		mr.version = MessageReceiptV1
		u = &messageReceiptV1{mr}
	default:
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// Ok to pass a CBOR reader since cbg.NewCborReader will return itself when
	// already a CBOR reader.
	return u.UnmarshalCBOR(cr)
}

var lengthBufAMessageReceiptV0 = []byte{131}

func (t *messageReceiptV0) MarshalCBOR(w io.Writer) error {
	// eliding null check since nulls were already handled in the dispatcher

	cw := cbg.NewCborWriter(w)

	if _, err := cw.Write(lengthBufAMessageReceiptV0); err != nil {
		return err
	}

	// t.ExitCode (exitcode.ExitCode) (int64)
	if t.ExitCode >= 0 {
		if err := cw.WriteMajorTypeHeader(cbg.MajUnsignedInt, uint64(t.ExitCode)); err != nil {
			return err
		}
	} else {
		if err := cw.WriteMajorTypeHeader(cbg.MajNegativeInt, uint64(-t.ExitCode-1)); err != nil {
			return err
		}
	}

	// t.Return ([]uint8) (slice)
	if len(t.Return) > cbg.ByteArrayMaxLen {
		return xerrors.Errorf("Byte array in field t.Return was too long")
	}

	if err := cw.WriteMajorTypeHeader(cbg.MajByteString, uint64(len(t.Return))); err != nil {
		return err
	}

	if _, err := cw.Write(t.Return[:]); err != nil {
		return err
	}

	// t.GasUsed (int64) (int64)
	if t.GasUsed >= 0 {
		if err := cw.WriteMajorTypeHeader(cbg.MajUnsignedInt, uint64(t.GasUsed)); err != nil {
			return err
		}
	} else {
		if err := cw.WriteMajorTypeHeader(cbg.MajNegativeInt, uint64(-t.GasUsed-1)); err != nil {
			return err
		}
	}
	return nil
}

func (t *messageReceiptV0) UnmarshalCBOR(r io.Reader) (err error) {
	cr := cbg.NewCborReader(r)

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
				return fmt.Errorf("int64 negative overflow")
			}
			extraI = -1 - extraI
		default:
			return fmt.Errorf("wrong type for int64 field: %d", maj)
		}

		t.ExitCode = exitcode.ExitCode(extraI)
	}
	// t.Return ([]uint8) (slice)

	maj, extra, err := cr.ReadHeader()
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
		t.Return = make([]uint8, extra)
	}

	if _, err := io.ReadFull(cr, t.Return[:]); err != nil {
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
				return fmt.Errorf("int64 negative overflow")
			}
			extraI = -1 - extraI
		default:
			return fmt.Errorf("wrong type for int64 field: %d", maj)
		}

		t.GasUsed = extraI
	}
	return nil
}

var lengthBufBMessageReceiptV1 = []byte{132}

func (t *messageReceiptV1) MarshalCBOR(w io.Writer) error {
	// eliding null check since nulls were already handled in the dispatcher

	cw := cbg.NewCborWriter(w)

	if _, err := cw.Write(lengthBufBMessageReceiptV1); err != nil {
		return err
	}

	// t.ExitCode (exitcode.ExitCode) (int64)
	if t.ExitCode >= 0 {
		if err := cw.WriteMajorTypeHeader(cbg.MajUnsignedInt, uint64(t.ExitCode)); err != nil {
			return err
		}
	} else {
		if err := cw.WriteMajorTypeHeader(cbg.MajNegativeInt, uint64(-t.ExitCode-1)); err != nil {
			return err
		}
	}

	// t.Return ([]uint8) (slice)
	if len(t.Return) > cbg.ByteArrayMaxLen {
		return xerrors.Errorf("Byte array in field t.Return was too long")
	}

	if err := cw.WriteMajorTypeHeader(cbg.MajByteString, uint64(len(t.Return))); err != nil {
		return err
	}

	if _, err := cw.Write(t.Return[:]); err != nil {
		return err
	}

	// t.GasUsed (int64) (int64)
	if t.GasUsed >= 0 {
		if err := cw.WriteMajorTypeHeader(cbg.MajUnsignedInt, uint64(t.GasUsed)); err != nil {
			return err
		}
	} else {
		if err := cw.WriteMajorTypeHeader(cbg.MajNegativeInt, uint64(-t.GasUsed-1)); err != nil {
			return err
		}
	}

	// t.EventsRoot (cid.Cid) (struct)

	if t.EventsRoot == nil {
		if _, err := cw.Write(cbg.CborNull); err != nil {
			return err
		}
	} else {
		if err := cbg.WriteCid(cw, *t.EventsRoot); err != nil {
			return xerrors.Errorf("failed to write cid field t.EventsRoot: %w", err)
		}
	}

	return nil
}

func (t *messageReceiptV1) UnmarshalCBOR(r io.Reader) (err error) {
	cr := cbg.NewCborReader(r)

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
				return fmt.Errorf("int64 negative overflow")
			}
			extraI = -1 - extraI
		default:
			return fmt.Errorf("wrong type for int64 field: %d", maj)
		}

		t.ExitCode = exitcode.ExitCode(extraI)
	}
	// t.Return ([]uint8) (slice)

	maj, extra, err := cr.ReadHeader()
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
		t.Return = make([]uint8, extra)
	}

	if _, err := io.ReadFull(cr, t.Return[:]); err != nil {
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
				return fmt.Errorf("int64 negative overflow")
			}
			extraI = -1 - extraI
		default:
			return fmt.Errorf("wrong type for int64 field: %d", maj)
		}

		t.GasUsed = extraI
	}
	// t.EventsRoot (cid.Cid) (struct)

	{

		b, err := cr.ReadByte()
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

			t.EventsRoot = &c
		}

	}
	return nil
}
