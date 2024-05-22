package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p"
	inet "github.com/libp2p/go-libp2p/core/network"
	"github.com/urfave/cli/v2"

	cborutil "github.com/filecoin-project/go-cbor-util"

	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/node/hello"
)

var resultCh chan bool

var helloCmd = &cli.Command{
	Name:        "hello",
	Description: "Get remote peer hello message by multiaddr",
	ArgsUsage:   "[peerMultiaddr|minerActorAddress]",
	Action: func(cctx *cli.Context) error {
		ctx := lcli.ReqContext(cctx)
		resultCh = make(chan bool, 1)
		pis, err := lcli.AddrInfoFromArg(ctx, cctx)
		if err != nil {
			return err
		}
		h, err := libp2p.New()
		if err != nil {
			return err
		}
		h.SetStreamHandler(hello.ProtocolID, HandleStream)
		err = h.Connect(ctx, pis[0])
		if err != nil {
			return err
		}
		ctx, done := context.WithTimeout(ctx, 5*time.Second)
		defer done()
		select {
		case <-resultCh:
		case <-ctx.Done():
			fmt.Println("can't get hello message, please try again")
		}
		return nil
	},
}

func HandleStream(s inet.Stream) {
	var hmsg hello.HelloMessage
	_ = s.SetReadDeadline(time.Now().Add(30 * time.Second))
	if err := cborutil.ReadCborRPC(s, &hmsg); err != nil {
		log.Infow("failed to read hello message, disconnecting", "error", err)
		_ = s.Conn().Close()
		return
	}
	data, err := json.Marshal(hmsg)
	if err != nil {
		return
	}
	fmt.Println(string(data))
	resultCh <- true
}
