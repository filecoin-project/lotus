package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	httpapi "github.com/ipfs/go-ipfs-http-client"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/chain/types"
)

func (h *dxhnd) handlePingMiner(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	vars := mux.Vars(r)
	a, err := address.NewFromString(vars["id"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	mi, err := h.api.StateMinerInfo(ctx, a, types.EmptyTSK)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if mi.PeerId == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	multiaddrs := make([]multiaddr.Multiaddr, 0, len(mi.Multiaddrs))
	for i, a := range mi.Multiaddrs {
		maddr, err := multiaddr.NewMultiaddrBytes(a)
		if err != nil {
			log.Warnf("parsing multiaddr %d (%x): %s", i, a, err)
			continue
		}
		multiaddrs = append(multiaddrs, maddr)
	}

	pi := peer.AddrInfo{
		ID:    *mi.PeerId,
		Addrs: multiaddrs,
	}

	{
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		if err := h.api.NetConnect(ctx, pi); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	d, err := h.api.NetPing(ctx, pi.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf("%s %s", pi.ID, d.Round(time.Millisecond))))
}

func (h *dxhnd) handlePingLotus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	vars := mux.Vars(r)

	aijson, err := base64.URLEncoding.DecodeString(vars["id"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pi := peer.AddrInfo{}
	if err := json.Unmarshal(aijson, &pi); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	{
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		if err := h.api.NetConnect(ctx, pi); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	d, err := h.api.NetPing(ctx, pi.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf("%s", d.Round(time.Millisecond))))
}

func (h *dxhnd) handlePingIPFS(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	vars := mux.Vars(r)

	aijson, err := base64.URLEncoding.DecodeString(vars["id"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pi := peer.AddrInfo{}
	if err := json.Unmarshal(aijson, &pi); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	iapi, err := httpapi.NewLocalApi()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	start := time.Now()

	// todo: not quite pinging

	if err := iapi.Swarm().Connect(ctx, pi); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	d := time.Now().Sub(start)

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf("%s", d.Round(time.Millisecond))))
}
