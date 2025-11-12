package blockstore

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/libp2p/go-msgio"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

type NetRPCReqType byte

const (
	NRpcHas NetRPCReqType = iota
	NRpcGet
	NRpcGetSize
	NRpcPut
	NRpcDelete

	// todo cancel req
)

type NetRPCRespType byte

const (
	NRpcOK NetRPCRespType = iota
	NRpcErr
	NRpcMore
)

type NetRPCErrType byte

const (
	NRpcErrGeneric NetRPCErrType = iota
	NRpcErrNotFound
)

type NetRpcReq struct {
	Type NetRPCReqType
	ID   uint64

	Cid  []cid.Cid // todo maxsize?
	Data [][]byte  // todo maxsize?
}

type NetRpcResp struct {
	Type NetRPCRespType
	ID   uint64

	// error or cids in allkeys
	Data []byte // todo maxsize?

	next <-chan NetRpcResp
}

type NetRpcErr struct {
	Type NetRPCErrType

	Msg string

	// in case of NRpcErrNotFound
	Cid *cid.Cid
}

type NetworkStore struct {
	// note: writer is thread-safe
	msgStream msgio.ReadWriteCloser

	// atomic
	reqCount uint64

	respLk sync.Mutex

	// respMap is nil after store closes
	respMap map[uint64]chan<- NetRpcResp

	closing chan struct{}
	closed  chan struct{}

	closeLk sync.Mutex
	onClose []func()
}

func NewNetworkStore(mss msgio.ReadWriteCloser) *NetworkStore {
	ns := &NetworkStore{
		msgStream: mss,

		respMap: map[uint64]chan<- NetRpcResp{},

		closing: make(chan struct{}),
		closed:  make(chan struct{}),
	}

	go ns.receive()

	return ns
}

func (n *NetworkStore) shutdown(msg string) {
	if err := n.msgStream.Close(); err != nil {
		log.Errorw("closing netstore msg stream", "error", err)
	}

	nerr := NetRpcErr{
		Type: NRpcErrGeneric,
		Msg:  msg,
		Cid:  nil,
	}

	var errb bytes.Buffer
	if err := nerr.MarshalCBOR(&errb); err != nil {
		log.Errorw("netstore shutdown: error marshaling error", "err", err)
	}

	n.respLk.Lock()
	for id, resps := range n.respMap {
		resps <- NetRpcResp{
			Type: NRpcErr,
			ID:   id,
			Data: errb.Bytes(),
		}
	}

	n.respMap = nil

	n.respLk.Unlock()
}

func (n *NetworkStore) OnClose(cb func()) {
	n.closeLk.Lock()
	defer n.closeLk.Unlock()

	select {
	case <-n.closed:
		cb()
	default:
		n.onClose = append(n.onClose, cb)
	}
}

func (n *NetworkStore) receive() {
	defer func() {
		n.closeLk.Lock()
		defer n.closeLk.Unlock()

		close(n.closed)
		if n.onClose != nil {
			for _, f := range n.onClose {
				f()
			}
		}
	}()

	for {
		select {
		case <-n.closing:
			n.shutdown("netstore stopping")
			return
		default:
		}

		msg, err := n.msgStream.ReadMsg()
		if err != nil {
			n.shutdown(fmt.Sprintf("netstore ReadMsg: %s", err))
			return
		}

		var resp NetRpcResp
		if err := resp.UnmarshalCBOR(bytes.NewReader(msg)); err != nil {
			n.shutdown(fmt.Sprintf("unmarshaling netstore response: %s", err))
			return
		}

		n.msgStream.ReleaseMsg(msg)

		n.respLk.Lock()
		if ch, ok := n.respMap[resp.ID]; ok {
			if resp.Type == NRpcMore {
				nch := make(chan NetRpcResp, 1)
				resp.next = nch
				n.respMap[resp.ID] = nch
			} else {
				delete(n.respMap, resp.ID)
			}

			ch <- resp
		}
		n.respLk.Unlock()
	}
}

func (n *NetworkStore) sendRpc(rt NetRPCReqType, cids []cid.Cid, data [][]byte) (uint64, <-chan NetRpcResp, error) {
	rid := atomic.AddUint64(&n.reqCount, 1)

	respCh := make(chan NetRpcResp, 1) // todo pool?

	n.respLk.Lock()
	if n.respMap == nil {
		n.respLk.Unlock()
		return 0, nil, xerrors.Errorf("netstore closed")
	}
	n.respMap[rid] = respCh
	n.respLk.Unlock()

	req := NetRpcReq{
		Type: rt,
		ID:   rid,
		Cid:  cids,
		Data: data,
	}

	var rbuf bytes.Buffer // todo buffer pool
	if err := req.MarshalCBOR(&rbuf); err != nil {
		n.respLk.Lock()
		defer n.respLk.Unlock()

		if n.respMap == nil {
			return 0, nil, xerrors.Errorf("netstore closed")
		}
		delete(n.respMap, rid)

		return 0, nil, err
	}

	if err := n.msgStream.WriteMsg(rbuf.Bytes()); err != nil {
		n.respLk.Lock()
		defer n.respLk.Unlock()

		if n.respMap == nil {
			return 0, nil, xerrors.Errorf("netstore closed")
		}
		delete(n.respMap, rid)

		return 0, nil, err
	}

	return rid, respCh, nil
}

func (n *NetworkStore) waitResp(ctx context.Context, rch <-chan NetRpcResp, rid uint64) (NetRpcResp, error) {
	select {
	case resp := <-rch:
		if resp.Type == NRpcErr {
			var e NetRpcErr
			if err := e.UnmarshalCBOR(bytes.NewReader(resp.Data)); err != nil {
				return NetRpcResp{}, xerrors.Errorf("unmarshaling error data: %w", err)
			}

			var err error
			switch e.Type {
			case NRpcErrNotFound:
				if e.Cid != nil {
					err = ipld.ErrNotFound{
						Cid: *e.Cid,
					}
				} else {
					err = xerrors.Errorf("block not found, but cid was null")
				}
			case NRpcErrGeneric:
				err = xerrors.Errorf("generic error")
			default:
				err = xerrors.Errorf("unknown error type")
			}

			return NetRpcResp{}, xerrors.Errorf("netstore error response: %s (%w)", e.Msg, err)
		}

		return resp, nil
	case <-ctx.Done():
		// todo send cancel req

		n.respLk.Lock()
		if n.respMap != nil {
			delete(n.respMap, rid)
		}
		n.respLk.Unlock()

		return NetRpcResp{}, ctx.Err()
	}
}

func (n *NetworkStore) Has(ctx context.Context, c cid.Cid) (bool, error) {
	req, rch, err := n.sendRpc(NRpcHas, []cid.Cid{c}, nil)
	if err != nil {
		return false, err
	}

	resp, err := n.waitResp(ctx, rch, req)
	if err != nil {
		return false, err
	}

	if len(resp.Data) != 1 {
		return false, xerrors.Errorf("expected reposnse length to be 1 byte")
	}
	switch resp.Data[0] {
	case cbg.CborBoolTrue[0]:
		return true, nil
	case cbg.CborBoolFalse[0]:
		return false, nil
	default:
		return false, xerrors.Errorf("has: bad response: %x", resp.Data[0])
	}
}

func (n *NetworkStore) Get(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	req, rch, err := n.sendRpc(NRpcGet, []cid.Cid{c}, nil)
	if err != nil {
		return nil, err
	}

	resp, err := n.waitResp(ctx, rch, req)
	if err != nil {
		return nil, err
	}

	return blocks.NewBlockWithCid(resp.Data, c)
}

func (n *NetworkStore) View(ctx context.Context, c cid.Cid, callback func([]byte) error) error {
	req, rch, err := n.sendRpc(NRpcGet, []cid.Cid{c}, nil)
	if err != nil {
		return err
	}

	resp, err := n.waitResp(ctx, rch, req)
	if err != nil {
		return err
	}

	return callback(resp.Data) // todo return buf to pool
}

func (n *NetworkStore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	req, rch, err := n.sendRpc(NRpcGetSize, []cid.Cid{c}, nil)
	if err != nil {
		return 0, err
	}

	resp, err := n.waitResp(ctx, rch, req)
	if err != nil {
		return 0, err
	}

	if len(resp.Data) != 4 {
		return 0, xerrors.Errorf("expected getsize response to be 4 bytes, was %d", resp.Data)
	}

	return int(binary.LittleEndian.Uint32(resp.Data)), nil
}

func (n *NetworkStore) Put(ctx context.Context, block blocks.Block) error {
	return n.PutMany(ctx, []blocks.Block{block})
}

func (n *NetworkStore) PutMany(ctx context.Context, blocks []blocks.Block) error {
	// todo pool
	cids := make([]cid.Cid, len(blocks))
	blkDatas := make([][]byte, len(blocks))
	for i, block := range blocks {
		cids[i] = block.Cid()
		blkDatas[i] = block.RawData()
	}

	req, rch, err := n.sendRpc(NRpcPut, cids, blkDatas)
	if err != nil {
		return err
	}

	_, err = n.waitResp(ctx, rch, req)
	if err != nil {
		return err
	}

	return nil
}

func (n *NetworkStore) DeleteBlock(ctx context.Context, c cid.Cid) error {
	return n.DeleteMany(ctx, []cid.Cid{c})
}

func (n *NetworkStore) DeleteMany(ctx context.Context, cids []cid.Cid) error {
	req, rch, err := n.sendRpc(NRpcDelete, cids, nil)
	if err != nil {
		return err
	}

	_, err = n.waitResp(ctx, rch, req)
	if err != nil {
		return err
	}

	return nil
}

func (n *NetworkStore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return nil, xerrors.Errorf("not supported")
}

func (*NetworkStore) Flush(context.Context) error { return nil }

func (n *NetworkStore) Stop(ctx context.Context) error {
	close(n.closing)

	select {
	case <-n.closed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

var _ Blockstore = &NetworkStore{}
