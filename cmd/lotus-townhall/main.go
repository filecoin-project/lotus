package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	rice "github.com/GeertJohan/go.rice"
	"github.com/gorilla/websocket"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/peer"
	pnet "github.com/libp2p/go-libp2p-pnet"
	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/filecoin-project/go-lotus/lib/addrutil"
	"github.com/filecoin-project/go-lotus/node/modules/lp2p"
)

const topic = "/fil/headnotifs/bafy2bzacea77zxnepp7wuqqgpj7xcw2ywwmmcmtrbjghhv4g2dildogpv6roi"

var upgrader = websocket.Upgrader{
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func main() {
	ctx := context.Background()

	protec, err := pnet.NewProtector(strings.NewReader(lp2p.LotusKey))
	if err != nil {
		panic(err)
	}

	host, err := libp2p.New(
		ctx,
		libp2p.Defaults,
		libp2p.PrivateNetwork(protec),
	)
	if err != nil {
		panic(err)
	}
	ps, err := pubsub.NewGossipSub(ctx, host)
	if err != nil {
		panic(err)
	}

	pi, err := addrutil.ParseAddresses(ctx, []string{
		"/ip4/147.75.80.29/tcp/1347/p2p/12D3KooWGU8C1mFsEtz4bXmHUH3kQTnQnxVy8cigwGV94qCpYJw7",
	})
	if err != nil {
		panic(err)
	}

	if err := host.Connect(ctx, pi[0]); err != nil {
		panic(err)
	}

	http.HandleFunc("/sub", handler(ps))
	http.Handle("/", http.FileServer(rice.MustFindBox("townhall/build").HTTPBox()))

	fmt.Println("listening on http://localhost:2975")

	if err := http.ListenAndServe("0.0.0.0:2975", nil); err != nil {
		panic(err)
	}
}

type update struct {
	From   peer.ID
	Update json.RawMessage
}

func handler(ps *pubsub.PubSub) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Header.Get("Sec-WebSocket-Protocol") != "" {
			w.Header().Set("Sec-WebSocket-Protocol", r.Header.Get("Sec-WebSocket-Protocol"))
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		sub, err := ps.Subscribe(topic)
		if err != nil {
			return
		}

		for {
			msg, err := sub.Next(r.Context())
			if err != nil {
				return
			}

			fmt.Println(msg)

			if err := conn.WriteJSON(update{
				From:   peer.ID(msg.From),
				Update: msg.Data,
			}); err != nil {
				return
			}
		}
	}
}
