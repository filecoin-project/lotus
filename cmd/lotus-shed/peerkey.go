package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/lp2p"
	"github.com/libp2p/go-libp2p-core/peer"
)

type keystore struct {
	set  bool
	info types.KeyInfo
}

func (ks *keystore) Put(name string, info types.KeyInfo) error {
	ks.info = info
	ks.set = true

	return nil
}

func (ks *keystore) Get(name string) (types.KeyInfo, error) {
	if !ks.set {
		return types.KeyInfo{}, types.ErrKeyInfoNotFound
	}

	return ks.info, nil
}

func (ks *keystore) Delete(name string) error {
	panic("Implement me")
}

func (ks *keystore) List() ([]string, error) {
	panic("Implement me")
}

var peerkeyCmd = &cli.Command{
	Name:        "peerkey",
	Description: "create libp2p host key",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "output",
			Value: "<peerid>.peerkey",
			Usage: "Output file format",
		},
		&cli.BoolFlag{
			Name:  "silent",
			Value: false,
			Usage: "Do not print peerid at end",
		},
	},
	Action: func(cctx *cli.Context) error {
		output := cctx.String("output")
		ks := keystore{}

		sk, err := lp2p.PrivKey(&ks)
		if err != nil {
			return err
		}

		bs, err := json.Marshal(ks.info)
		if err != nil {
			return err
		}

		peerid, err := peer.IDFromPrivateKey(sk)
		if err != nil {
			return err
		}

		output = strings.ReplaceAll(output, "<peerid>", peerid.String())

		f, err := os.Create(output)
		if err != nil {
			return err
		}

		defer func() {
			if err := f.Close(); err != nil {
				log.Warnf("failed to close output file: %w", err)
			}
		}()

		if _, err := f.Write(bs); err != nil {
			return err
		}

		if !cctx.Bool("silent") {
			fmt.Println(peerid.String())
		}

		return nil
	},
}
