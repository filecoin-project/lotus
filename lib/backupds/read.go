package backupds

import (
	"io"

	"github.com/ipfs/go-datastore"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

func ReadBackup(r io.Reader, cb func(key datastore.Key, value []byte) error) error {
	scratch := make([]byte, 9)

	if _, err := r.Read(scratch[:1]); err != nil {
		return xerrors.Errorf("reading array header: %w", err)
	}

	if scratch[0] != 0x9f {
		return xerrors.Errorf("expected indefinite length array header byte 0x9f, got %x", scratch[0])
	}

	for {
		if _, err := r.Read(scratch[:1]); err != nil {
			return xerrors.Errorf("reading tuple header: %w", err)
		}

		if scratch[0] == 0xff {
			break
		}

		if scratch[0] != 0x82 {
			return xerrors.Errorf("expected array(2) header 0x82, got %x", scratch[0])
		}

		keyb, err := cbg.ReadByteArray(r, 1<<40)
		if err != nil {
			return xerrors.Errorf("reading key: %w", err)
		}
		key := datastore.NewKey(string(keyb))

		value, err := cbg.ReadByteArray(r, 1<<40)
		if err != nil {
			return xerrors.Errorf("reading value: %w", err)
		}

		if err := cb(key, value); err != nil {
			return err
		}
	}

	return nil
}
