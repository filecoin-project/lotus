package backupds

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"os"

	"github.com/ipfs/go-datastore"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

func ReadBackup(r io.Reader, cb func(key datastore.Key, value []byte, log bool) error) (bool, error) {
	scratch := make([]byte, 9)

	// read array[2](
	if _, err := r.Read(scratch[:1]); err != nil {
		return false, xerrors.Errorf("reading array header: %w", err)
	}

	if scratch[0] != 0x82 {
		return false, xerrors.Errorf("expected array(2) header byte 0x82, got %x", scratch[0])
	}

	hasher := sha256.New()
	hr := io.TeeReader(r, hasher)

	// read array[*](
	if _, err := hr.Read(scratch[:1]); err != nil {
		return false, xerrors.Errorf("reading array header: %w", err)
	}

	if scratch[0] != 0x9f {
		return false, xerrors.Errorf("expected indefinite length array header byte 0x9f, got %x", scratch[0])
	}

	for {
		if _, err := hr.Read(scratch[:1]); err != nil {
			return false, xerrors.Errorf("reading tuple header: %w", err)
		}

		// close array[*]
		if scratch[0] == 0xff {
			break
		}

		// read array[2](key:[]byte, value:[]byte)
		if scratch[0] != 0x82 {
			return false, xerrors.Errorf("expected array(2) header 0x82, got %x", scratch[0])
		}

		keyb, err := cbg.ReadByteArray(hr, 1<<40)
		if err != nil {
			return false, xerrors.Errorf("reading key: %w", err)
		}
		key := datastore.NewKey(string(keyb))

		value, err := cbg.ReadByteArray(hr, 1<<40)
		if err != nil {
			return false, xerrors.Errorf("reading value: %w", err)
		}

		if err := cb(key, value, false); err != nil {
			return false, err
		}
	}

	sum := hasher.Sum(nil)

	// read the [32]byte checksum
	expSum, err := cbg.ReadByteArray(r, 32)
	if err != nil {
		return false, xerrors.Errorf("reading expected checksum: %w", err)
	}

	if !bytes.Equal(sum, expSum) {
		return false, xerrors.Errorf("checksum didn't match; expected %x, got %x", expSum, sum)
	}

	// read the log, set of Entry-ies

	var ent Entry
	bp := cbg.GetPeeker(r)
	for {
		_, err := bp.ReadByte()
		switch err {
		case io.EOF, io.ErrUnexpectedEOF:
			return true, nil
		case nil:
		default:
			return false, xerrors.Errorf("peek log: %w", err)
		}
		if err := bp.UnreadByte(); err != nil {
			return false, xerrors.Errorf("unread log byte: %w", err)
		}

		if err := ent.UnmarshalCBOR(bp); err != nil {
			switch err {
			case io.EOF, io.ErrUnexpectedEOF:
				if os.Getenv("LOTUS_ALLOW_TRUNCATED_LOG") == "1" {
					log.Errorw("log entry potentially truncated")
					return false, nil
				}
				return false, xerrors.Errorf("log entry potentially truncated, set LOTUS_ALLOW_TRUNCATED_LOG=1 to proceed: %w", err)
			default:
				return false, xerrors.Errorf("unmarshaling log entry: %w", err)
			}
		}

		key := datastore.NewKey(string(ent.Key))

		if err := cb(key, ent.Value, true); err != nil {
			return false, err
		}
	}
}

func RestoreInto(r io.Reader, dest datastore.Batching) error {
	batch, err := dest.Batch(context.TODO())
	if err != nil {
		return xerrors.Errorf("creating batch: %w", err)
	}

	_, err = ReadBackup(r, func(key datastore.Key, value []byte, _ bool) error {
		if err := batch.Put(context.TODO(), key, value); err != nil {
			return xerrors.Errorf("put key: %w", err)
		}

		return nil
	})
	if err != nil {
		return xerrors.Errorf("reading backup: %w", err)
	}

	if err := batch.Commit(context.TODO()); err != nil {
		return xerrors.Errorf("committing batch: %w", err)
	}

	return nil
}
