package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"os"
	"strings"

	"github.com/dgraph-io/badger/v2/y"
	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/mitchellh/go-homedir"
	"github.com/multiformats/go-base32"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var datastoreVlog2CarCmd = &cli.Command{
	Name:  "vlog2car",
	Usage: "convert badger blockstore .vlog to .car",
	Flags: []cli.Flag{
		&cli.PathFlag{
			Name:     "car",
			Usage:    "out car file name (no .car)",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "key-prefix",
			Usage: "datastore prefix",
			Value: "/blocks/",
		},
		&cli.Uint64Flag{
			Name:  "max-size",
			Value: 32000,
			Usage: "max single car size in MiB",
		},
	},
	ArgsUsage: "[vlog...]",
	Action: func(cctx *cli.Context) error {
		ctx := cctx.Context

		maxSz := cctx.Uint64("max-size") << 20

		carb := &rawCarb{
			max:    maxSz,
			blocks: map[cid.Cid]block.Block{},
		}
		cars := 0

		pref := cctx.String("key-prefix")
		plen := len(pref)

		for _, vlogPath := range cctx.Args().Slice() {
			vlogPath, err := homedir.Expand(vlogPath)
			if err != nil {
				return xerrors.Errorf("expand vlog path: %w", err)
			}

			// NOTE: Some bits of code in this code block come from https://github.com/dgraph-io/badger, which is licensed
			//  under Apache 2.0; See https://github.com/dgraph-io/badger/blob/master/LICENSE

			vf, err := os.Open(vlogPath)
			if err != nil {
				return xerrors.Errorf("open vlog file: %w", err)
			}

			if _, err := vf.Seek(20, io.SeekStart); err != nil {
				return xerrors.Errorf("seek past vlog start: %w", err)
			}

			reader := bufio.NewReader(vf)
			read := &safeRead{
				k:            make([]byte, 10),
				v:            make([]byte, 10),
				recordOffset: 20,
			}

		loop:
			for {
				e, err := read.Entry(reader)
				switch {
				case err == io.EOF:
					break loop
				case err == io.ErrUnexpectedEOF || err == errTruncate:
					break loop
				case err != nil:
					return xerrors.Errorf("entry read error: %w", err)
				case e == nil:
					continue
				}

				if e.meta&0x40 > 0 {
					e.Key = e.Key[:len(e.Key)-8]
				} else if e.meta > 0 {
					if e.meta&0x3f > 0 {
						log.Infof("unk meta m:%x; k:%x, v:%60x", e.meta, e.Key, e.Value)
					}
					continue
				}

				{
					if plen > 0 && !strings.HasPrefix(string(e.Key), pref) {
						log.Infow("no blocks prefix", "key", string(e.Key))
						continue
					}

					h, err := base32.RawStdEncoding.DecodeString(string(e.Key[plen:]))
					if err != nil {
						return xerrors.Errorf("decode b32 ds key %x: %w", e.Key, err)
					}

					c := cid.NewCidV1(cid.Raw, h)

					b, err := block.NewBlockWithCid(e.Value, c)
					if err != nil {
						return xerrors.Errorf("readblk: %w", err)
					}

					err = carb.consume(c, b)
					switch err {
					case nil:
					case errFullCar:
						root, err := carb.finalize()
						if err != nil {
							return xerrors.Errorf("carb finalize: %w", err)
						}

						if err := carb.writeCar(ctx, fmt.Sprintf("%s%d.car", cctx.Path("car"), cars), root); err != nil {
							return xerrors.Errorf("writeCar: %w", err)
						}

						cars++

						carb = &rawCarb{
							max:    maxSz,
							blocks: map[cid.Cid]block.Block{},
						}

					default:
						return xerrors.Errorf("carb consume: %w", err)
					}
				}
			}

			if err := vf.Close(); err != nil {
				return err
			}
		}

		root, err := carb.finalize()
		if err != nil {
			return xerrors.Errorf("carb finalize: %w", err)
		}

		if err := carb.writeCar(ctx, fmt.Sprintf("%s%d.car", cctx.Path("car"), cars), root); err != nil {
			return xerrors.Errorf("writeCar: %w", err)
		}

		return nil

	},
}

// NOTE: Code below comes (with slight modifications) from https://github.com/dgraph-io/badger/blob/master/value.go
// Apache 2.0; See https://github.com/dgraph-io/badger/blob/master/LICENSE

var errTruncate = errors.New("do truncate")

// hashReader implements io.Reader, io.ByteReader interfaces. It also keeps track of the number
// bytes read. The hashReader writes to h (hash) what it reads from r.
type hashReader struct {
	r         io.Reader
	h         hash.Hash32
	bytesRead int // Number of bytes read.
}

func newHashReader(r io.Reader) *hashReader {
	hash := crc32.New(y.CastagnoliCrcTable)
	return &hashReader{
		r: r,
		h: hash,
	}
}

// Read reads len(p) bytes from the reader. Returns the number of bytes read, error on failure.
func (t *hashReader) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if err != nil {
		return n, err
	}
	t.bytesRead += n
	return t.h.Write(p[:n])
}

// ReadByte reads exactly one byte from the reader. Returns error on failure.
func (t *hashReader) ReadByte() (byte, error) {
	b := make([]byte, 1)
	_, err := t.Read(b)
	return b[0], err
}

// Sum32 returns the sum32 of the underlying hash.
func (t *hashReader) Sum32() uint32 {
	return t.h.Sum32()
}

type safeRead struct {
	k []byte
	v []byte

	recordOffset uint32
}

// Entry provides Key, Value, UserMeta and ExpiresAt. This struct can be used by
// the user to set data.
type Entry struct {
	Key       []byte
	Value     []byte
	UserMeta  byte
	ExpiresAt uint64 // time.Unix
	meta      byte

	// Fields maintained internally.
	offset uint32
	hlen   int // Length of the header.
}

// Entry reads an entry from the provided reader. It also validates the checksum for every entry
// read. Returns error on failure.
func (r *safeRead) Entry(reader io.Reader) (*Entry, error) {
	tee := newHashReader(reader)
	var h header
	hlen, err := h.DecodeFrom(tee)
	if err != nil {
		return nil, err
	}
	if h.klen > uint32(1<<16) { // Key length must be below uint16.
		return nil, errTruncate
	}
	kl := int(h.klen)
	if cap(r.k) < kl {
		r.k = make([]byte, 2*kl)
	}
	vl := int(h.vlen)
	if cap(r.v) < vl {
		r.v = make([]byte, 2*vl)
	}

	e := &Entry{}
	e.offset = r.recordOffset
	e.hlen = hlen
	buf := make([]byte, h.klen+h.vlen)
	if _, err := io.ReadFull(tee, buf[:]); err != nil {
		if err == io.EOF {
			err = errTruncate
		}
		return nil, err
	}
	e.Key = buf[:h.klen]
	e.Value = buf[h.klen:]
	var crcBuf [crc32.Size]byte
	if _, err := io.ReadFull(reader, crcBuf[:]); err != nil {
		if err == io.EOF {
			err = errTruncate
		}
		return nil, err
	}
	crc := y.BytesToU32(crcBuf[:])
	if crc != tee.Sum32() {
		return nil, errTruncate
	}
	e.meta = h.meta
	e.UserMeta = h.userMeta
	e.ExpiresAt = h.expiresAt
	return e, nil
}

// header is used in value log as a header before Entry.
type header struct {
	klen      uint32
	vlen      uint32
	expiresAt uint64
	meta      byte
	userMeta  byte
}

// Encode encodes the header into []byte. The provided []byte should be at least 5 bytes. The
// function will panic if out []byte isn't large enough to hold all the values.
// The encoded header looks like
// +------+----------+------------+--------------+-----------+
// | Meta | UserMeta | Key Length | Value Length | ExpiresAt |
// +------+----------+------------+--------------+-----------+
func (h header) Encode(out []byte) int {
	out[0], out[1] = h.meta, h.userMeta
	index := 2
	index += binary.PutUvarint(out[index:], uint64(h.klen))
	index += binary.PutUvarint(out[index:], uint64(h.vlen))
	index += binary.PutUvarint(out[index:], h.expiresAt)
	return index
}

// Decode decodes the given header from the provided byte slice.
// Returns the number of bytes read.
func (h *header) Decode(buf []byte) int {
	h.meta, h.userMeta = buf[0], buf[1]
	index := 2
	klen, count := binary.Uvarint(buf[index:])
	h.klen = uint32(klen)
	index += count
	vlen, count := binary.Uvarint(buf[index:])
	h.vlen = uint32(vlen)
	index += count
	h.expiresAt, count = binary.Uvarint(buf[index:])
	return index + count
}

// DecodeFrom reads the header from the hashReader.
// Returns the number of bytes read.
func (h *header) DecodeFrom(reader *hashReader) (int, error) {
	var err error
	h.meta, err = reader.ReadByte()
	if err != nil {
		return 0, err
	}
	h.userMeta, err = reader.ReadByte()
	if err != nil {
		return 0, err
	}
	klen, err := binary.ReadUvarint(reader)
	if err != nil {
		return 0, err
	}
	h.klen = uint32(klen)
	vlen, err := binary.ReadUvarint(reader)
	if err != nil {
		return 0, err
	}
	h.vlen = uint32(vlen)
	h.expiresAt, err = binary.ReadUvarint(reader)
	if err != nil {
		return 0, err
	}
	return reader.bytesRead, nil
}
