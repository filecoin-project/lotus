package backupds

import (
	"bytes"
	"crypto/sha256"
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

	if scratch[0] != 0x82 {
		return xerrors.Errorf("expected array(2) header byte 0x82, got %x", scratch[0])
	}

	hasher := sha256.New()
	hr := io.TeeReader(r, hasher)

	if _, err := hr.Read(scratch[:1]); err != nil {
		return xerrors.Errorf("reading array header: %w", err)
	}

	if scratch[0] != 0x9f {
		return xerrors.Errorf("expected indefinite length array header byte 0x9f, got %x", scratch[0])
	}

	for {
		if _, err := hr.Read(scratch[:1]); err != nil {
			return xerrors.Errorf("reading tuple header: %w", err)
		}

		if scratch[0] == 0xff {
			break
		}

		if scratch[0] != 0x82 {
			return xerrors.Errorf("expected array(2) header 0x82, got %x", scratch[0])
		}

		keyb, err := cbg.ReadByteArray(hr, 1<<40)
		if err != nil {
			return xerrors.Errorf("reading key: %w", err)
		}
		key := datastore.NewKey(string(keyb))

		value, err := cbg.ReadByteArray(hr, 1<<40)
		if err != nil {
			return xerrors.Errorf("reading value: %w", err)
		}

		if err := cb(key, value); err != nil {
			return err
		}
	}

	sum := hasher.Sum(nil)

	expSum, err := cbg.ReadByteArray(r, 32)
	if err != nil {
		return xerrors.Errorf("reading expected checksum: %w", err)
	}

	if !bytes.Equal(sum, expSum) {
		return xerrors.Errorf("checksum didn't match; expected %x, got %x", expSum, sum)
	}

	return nil
}

func RestoreInto(r io.Reader, dest datastore.Batching) error {
	batch, err := dest.Batch()
	if err != nil {
		return xerrors.Errorf("creating batch: %w", err)
	}

	err = ReadBackup(r, func(key datastore.Key, value []byte) error {
		if err := batch.Put(key, value); err != nil {
			return xerrors.Errorf("put key: %w", err)
		}

		return nil
	})
	if err != nil {
		return xerrors.Errorf("reading backup: %w", err)
	}

	if err := batch.Commit(); err != nil {
		return xerrors.Errorf("committing batch: %w", err)
	}

	return nil
}
