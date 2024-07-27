package blockstore

import (
	"bytes"
	"context"
	"encoding/binary"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/libp2p/go-msgio"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

type NetworkStoreHandler struct {
	msgStream msgio.ReadWriteCloser

	bs Blockstore
}

// NOTE: This code isn't yet hardened to accept untrusted input. See TODOs here and in net.go

func HandleNetBstoreStream(ctx context.Context, bs Blockstore, mss msgio.ReadWriteCloser) *NetworkStoreHandler {
	ns := &NetworkStoreHandler{
		msgStream: mss,
		bs:        bs,
	}

	go ns.handle(ctx)

	return ns
}

func (h *NetworkStoreHandler) handle(ctx context.Context) {
	defer func() {
		if err := h.msgStream.Close(); err != nil {
			log.Errorw("error closing blockstore stream", "error", err)
		}
	}()

	for {
		var req NetRpcReq

		ms, err := h.msgStream.ReadMsg()
		if err != nil {
			log.Warnw("bstore stream err", "error", err)
			return
		}

		if err := req.UnmarshalCBOR(bytes.NewReader(ms)); err != nil {
			return
		}

		h.msgStream.ReleaseMsg(ms)

		switch req.Type {
		case NRpcHas:
			if len(req.Cid) != 1 {
				if err := h.respondError(req.ID, xerrors.New("expected request for 1 cid"), cid.Undef); err != nil {
					log.Warnw("writing error response", "error", err)
					return
				}
				continue
			}

			res, err := h.bs.Has(ctx, req.Cid[0])
			if err != nil {
				if err := h.respondError(req.ID, err, req.Cid[0]); err != nil {
					log.Warnw("writing error response", "error", err)
					return
				}
				continue
			}

			var resData [1]byte
			if res {
				resData[0] = cbg.CborBoolTrue[0]
			} else {
				resData[0] = cbg.CborBoolFalse[0]
			}

			if err := h.respond(req.ID, NRpcOK, resData[:]); err != nil {
				log.Warnw("writing response", "error", err)
				return
			}

		case NRpcGet:
			if len(req.Cid) != 1 {
				if err := h.respondError(req.ID, xerrors.New("expected request for 1 cid"), cid.Undef); err != nil {
					log.Warnw("writing error response", "error", err)
					return
				}
				continue
			}

			err := h.bs.View(ctx, req.Cid[0], func(bdata []byte) error {
				return h.respond(req.ID, NRpcOK, bdata)
			})
			if err != nil {
				if err := h.respondError(req.ID, err, req.Cid[0]); err != nil {
					log.Warnw("writing error response", "error", err)
					return
				}
				continue
			}

		case NRpcGetSize:
			if len(req.Cid) != 1 {
				if err := h.respondError(req.ID, xerrors.New("expected request for 1 cid"), cid.Undef); err != nil {
					log.Warnw("writing error response", "error", err)
					return
				}
				continue
			}

			sz, err := h.bs.GetSize(ctx, req.Cid[0])
			if err != nil {
				if err := h.respondError(req.ID, err, req.Cid[0]); err != nil {
					log.Warnw("writing error response", "error", err)
					return
				}
				continue
			}

			var resData [4]byte
			binary.LittleEndian.PutUint32(resData[:], uint32(sz))

			if err := h.respond(req.ID, NRpcOK, resData[:]); err != nil {
				log.Warnw("writing response", "error", err)
				return
			}

		case NRpcPut:
			blocks := make([]block.Block, len(req.Cid))

			if len(req.Cid) != len(req.Data) {
				if err := h.respondError(req.ID, xerrors.New("cid count didn't match data count"), cid.Undef); err != nil {
					log.Warnw("writing error response", "error", err)
				}
				return
			}

			for i := range req.Cid {
				blocks[i], err = block.NewBlockWithCid(req.Data[i], req.Cid[i])
				if err != nil {
					log.Warnw("make block", "error", err)
					return
				}
			}

			err := h.bs.PutMany(ctx, blocks)
			if err != nil {
				if err := h.respondError(req.ID, err, cid.Undef); err != nil {
					log.Warnw("writing error response", "error", err)
					return
				}
				continue
			}

			if err := h.respond(req.ID, NRpcOK, []byte{}); err != nil {
				log.Warnw("writing response", "error", err)
				return
			}
		case NRpcDelete:
			err := h.bs.DeleteMany(ctx, req.Cid)
			if err != nil {
				if err := h.respondError(req.ID, err, cid.Undef); err != nil {
					log.Warnw("writing error response", "error", err)
					return
				}
				continue
			}

			if err := h.respond(req.ID, NRpcOK, []byte{}); err != nil {
				log.Warnw("writing response", "error", err)
				return
			}
		default:
			if err := h.respondError(req.ID, xerrors.New("unsupported request type"), cid.Undef); err != nil {
				log.Warnw("writing error response", "error", err)
				return
			}
			continue
		}
	}
}

func (h *NetworkStoreHandler) respondError(req uint64, uerr error, c cid.Cid) error {
	var resp NetRpcResp
	resp.ID = req
	resp.Type = NRpcErr

	nerr := NetRpcErr{
		Type: NRpcErrGeneric,
		Msg:  uerr.Error(),
	}
	if ipld.IsNotFound(uerr) {
		nerr.Type = NRpcErrNotFound
		nerr.Cid = &c
	}

	var edata bytes.Buffer
	if err := nerr.MarshalCBOR(&edata); err != nil {
		return xerrors.Errorf("marshaling error data: %w", err)
	}

	resp.Data = edata.Bytes()

	var msg bytes.Buffer
	if err := resp.MarshalCBOR(&msg); err != nil {
		return xerrors.Errorf("marshaling error response: %w", err)
	}

	if err := h.msgStream.WriteMsg(msg.Bytes()); err != nil {
		return xerrors.Errorf("write error response: %w", err)
	}

	return nil
}

func (h *NetworkStoreHandler) respond(req uint64, rt NetRPCRespType, data []byte) error {
	var resp NetRpcResp
	resp.ID = req
	resp.Type = rt
	resp.Data = data

	var msg bytes.Buffer
	if err := resp.MarshalCBOR(&msg); err != nil {
		return xerrors.Errorf("marshaling response: %w", err)
	}

	if err := h.msgStream.WriteMsg(msg.Bytes()); err != nil {
		return xerrors.Errorf("write response: %w", err)
	}

	return nil
}
