// Code generated by github.com/whyrusleeping/cbor-gen. DO NOT EDIT.

package storiface

import (
	"fmt"
	"io"
	"math"
	"sort"

	cid "github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	xerrors "golang.org/x/xerrors"
)

var _ = xerrors.Errorf
var _ = cid.Undef
var _ = math.E
var _ = sort.Sort

func (t *CallID) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}

	cw := cbg.NewCborWriter(w)

	if _, err := cw.Write([]byte{162}); err != nil {
		return err
	}

	// t.ID (uuid.UUID) (array)
	if len("ID") > 8192 {
		return xerrors.Errorf("Value in field \"ID\" was too long")
	}

	if err := cw.WriteMajorTypeHeader(cbg.MajTextString, uint64(len("ID"))); err != nil {
		return err
	}
	if _, err := cw.WriteString(string("ID")); err != nil {
		return err
	}

	if len(t.ID) > 2097152 {
		return xerrors.Errorf("Byte array in field t.ID was too long")
	}

	if err := cw.WriteMajorTypeHeader(cbg.MajByteString, uint64(len(t.ID))); err != nil {
		return err
	}

	if _, err := cw.Write(t.ID[:]); err != nil {
		return err
	}

	// t.Sector (abi.SectorID) (struct)
	if len("Sector") > 8192 {
		return xerrors.Errorf("Value in field \"Sector\" was too long")
	}

	if err := cw.WriteMajorTypeHeader(cbg.MajTextString, uint64(len("Sector"))); err != nil {
		return err
	}
	if _, err := cw.WriteString(string("Sector")); err != nil {
		return err
	}

	if err := t.Sector.MarshalCBOR(cw); err != nil {
		return err
	}
	return nil
}

func (t *CallID) UnmarshalCBOR(r io.Reader) (err error) {
	*t = CallID{}

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

	if maj != cbg.MajMap {
		return fmt.Errorf("cbor input should be of type map")
	}

	if extra > cbg.MaxLength {
		return fmt.Errorf("CallID: map struct too large (%d)", extra)
	}

	var name string
	n := extra

	for i := uint64(0); i < n; i++ {

		{
			sval, err := cbg.ReadStringWithMax(cr, 8192)
			if err != nil {
				return err
			}

			name = string(sval)
		}

		switch name {
		// t.ID (uuid.UUID) (array)
		case "ID":

			maj, extra, err = cr.ReadHeader()
			if err != nil {
				return err
			}

			if extra > 2097152 {
				return fmt.Errorf("t.ID: byte array too large (%d)", extra)
			}
			if maj != cbg.MajByteString {
				return fmt.Errorf("expected byte array")
			}
			if extra != 16 {
				return fmt.Errorf("expected array to have 16 elements")
			}

			t.ID = [16]uint8{}
			if _, err := io.ReadFull(cr, t.ID[:]); err != nil {
				return err
			}
			// t.Sector (abi.SectorID) (struct)
		case "Sector":

			{

				if err := t.Sector.UnmarshalCBOR(cr); err != nil {
					return xerrors.Errorf("unmarshaling t.Sector: %w", err)
				}

			}

		default:
			// Field doesn't exist on this type, so ignore it
			cbg.ScanForLinks(r, func(cid.Cid) {})
		}
	}

	return nil
}
func (t *SecDataHttpHeader) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}

	cw := cbg.NewCborWriter(w)

	if _, err := cw.Write([]byte{162}); err != nil {
		return err
	}

	// t.Key (string) (string)
	if len("Key") > 8192 {
		return xerrors.Errorf("Value in field \"Key\" was too long")
	}

	if err := cw.WriteMajorTypeHeader(cbg.MajTextString, uint64(len("Key"))); err != nil {
		return err
	}
	if _, err := cw.WriteString(string("Key")); err != nil {
		return err
	}

	if len(t.Key) > 8192 {
		return xerrors.Errorf("Value in field t.Key was too long")
	}

	if err := cw.WriteMajorTypeHeader(cbg.MajTextString, uint64(len(t.Key))); err != nil {
		return err
	}
	if _, err := cw.WriteString(string(t.Key)); err != nil {
		return err
	}

	// t.Value (string) (string)
	if len("Value") > 8192 {
		return xerrors.Errorf("Value in field \"Value\" was too long")
	}

	if err := cw.WriteMajorTypeHeader(cbg.MajTextString, uint64(len("Value"))); err != nil {
		return err
	}
	if _, err := cw.WriteString(string("Value")); err != nil {
		return err
	}

	if len(t.Value) > 8192 {
		return xerrors.Errorf("Value in field t.Value was too long")
	}

	if err := cw.WriteMajorTypeHeader(cbg.MajTextString, uint64(len(t.Value))); err != nil {
		return err
	}
	if _, err := cw.WriteString(string(t.Value)); err != nil {
		return err
	}
	return nil
}

func (t *SecDataHttpHeader) UnmarshalCBOR(r io.Reader) (err error) {
	*t = SecDataHttpHeader{}

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

	if maj != cbg.MajMap {
		return fmt.Errorf("cbor input should be of type map")
	}

	if extra > cbg.MaxLength {
		return fmt.Errorf("SecDataHttpHeader: map struct too large (%d)", extra)
	}

	var name string
	n := extra

	for i := uint64(0); i < n; i++ {

		{
			sval, err := cbg.ReadStringWithMax(cr, 8192)
			if err != nil {
				return err
			}

			name = string(sval)
		}

		switch name {
		// t.Key (string) (string)
		case "Key":

			{
				sval, err := cbg.ReadStringWithMax(cr, 8192)
				if err != nil {
					return err
				}

				t.Key = string(sval)
			}
			// t.Value (string) (string)
		case "Value":

			{
				sval, err := cbg.ReadStringWithMax(cr, 8192)
				if err != nil {
					return err
				}

				t.Value = string(sval)
			}

		default:
			// Field doesn't exist on this type, so ignore it
			cbg.ScanForLinks(r, func(cid.Cid) {})
		}
	}

	return nil
}
func (t *SectorLocation) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}

	cw := cbg.NewCborWriter(w)

	if _, err := cw.Write([]byte{163}); err != nil {
		return err
	}

	// t.URL (string) (string)
	if len("URL") > 8192 {
		return xerrors.Errorf("Value in field \"URL\" was too long")
	}

	if err := cw.WriteMajorTypeHeader(cbg.MajTextString, uint64(len("URL"))); err != nil {
		return err
	}
	if _, err := cw.WriteString(string("URL")); err != nil {
		return err
	}

	if len(t.URL) > 8192 {
		return xerrors.Errorf("Value in field t.URL was too long")
	}

	if err := cw.WriteMajorTypeHeader(cbg.MajTextString, uint64(len(t.URL))); err != nil {
		return err
	}
	if _, err := cw.WriteString(string(t.URL)); err != nil {
		return err
	}

	// t.Local (bool) (bool)
	if len("Local") > 8192 {
		return xerrors.Errorf("Value in field \"Local\" was too long")
	}

	if err := cw.WriteMajorTypeHeader(cbg.MajTextString, uint64(len("Local"))); err != nil {
		return err
	}
	if _, err := cw.WriteString(string("Local")); err != nil {
		return err
	}

	if err := cbg.WriteBool(w, t.Local); err != nil {
		return err
	}

	// t.Headers ([]storiface.SecDataHttpHeader) (slice)
	if len("Headers") > 8192 {
		return xerrors.Errorf("Value in field \"Headers\" was too long")
	}

	if err := cw.WriteMajorTypeHeader(cbg.MajTextString, uint64(len("Headers"))); err != nil {
		return err
	}
	if _, err := cw.WriteString(string("Headers")); err != nil {
		return err
	}

	if len(t.Headers) > 8192 {
		return xerrors.Errorf("Slice value in field t.Headers was too long")
	}

	if err := cw.WriteMajorTypeHeader(cbg.MajArray, uint64(len(t.Headers))); err != nil {
		return err
	}
	for _, v := range t.Headers {
		if err := v.MarshalCBOR(cw); err != nil {
			return err
		}

	}
	return nil
}

func (t *SectorLocation) UnmarshalCBOR(r io.Reader) (err error) {
	*t = SectorLocation{}

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

	if maj != cbg.MajMap {
		return fmt.Errorf("cbor input should be of type map")
	}

	if extra > cbg.MaxLength {
		return fmt.Errorf("SectorLocation: map struct too large (%d)", extra)
	}

	var name string
	n := extra

	for i := uint64(0); i < n; i++ {

		{
			sval, err := cbg.ReadStringWithMax(cr, 8192)
			if err != nil {
				return err
			}

			name = string(sval)
		}

		switch name {
		// t.URL (string) (string)
		case "URL":

			{
				sval, err := cbg.ReadStringWithMax(cr, 8192)
				if err != nil {
					return err
				}

				t.URL = string(sval)
			}
			// t.Local (bool) (bool)
		case "Local":

			maj, extra, err = cr.ReadHeader()
			if err != nil {
				return err
			}
			if maj != cbg.MajOther {
				return fmt.Errorf("booleans must be major type 7")
			}
			switch extra {
			case 20:
				t.Local = false
			case 21:
				t.Local = true
			default:
				return fmt.Errorf("booleans are either major type 7, value 20 or 21 (got %d)", extra)
			}
			// t.Headers ([]storiface.SecDataHttpHeader) (slice)
		case "Headers":

			maj, extra, err = cr.ReadHeader()
			if err != nil {
				return err
			}

			if extra > 8192 {
				return fmt.Errorf("t.Headers: array too large (%d)", extra)
			}

			if maj != cbg.MajArray {
				return fmt.Errorf("expected cbor array")
			}

			if extra > 0 {
				t.Headers = make([]SecDataHttpHeader, extra)
			}

			for i := 0; i < int(extra); i++ {
				{
					var maj byte
					var extra uint64
					var err error
					_ = maj
					_ = extra
					_ = err

					{

						if err := t.Headers[i].UnmarshalCBOR(cr); err != nil {
							return xerrors.Errorf("unmarshaling t.Headers[i]: %w", err)
						}

					}

				}
			}

		default:
			// Field doesn't exist on this type, so ignore it
			cbg.ScanForLinks(r, func(cid.Cid) {})
		}
	}

	return nil
}
