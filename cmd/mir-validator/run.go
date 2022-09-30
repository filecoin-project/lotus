package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/ioutil"
	_ "net/http/pprof"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/ulimit"
	"github.com/filecoin-project/lotus/metrics"
)

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "Start a lotus miner process",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "default-key",
			Value: true,
			Usage: "use default wallet's key",
		},
		&cli.BoolFlag{
			Name:  "nosync",
			Usage: "don't check full-node sync status",
		},
		&cli.BoolFlag{
			Name:  "manage-fdlimit",
			Usage: "manage open file limit",
			Value: true,
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Bool("enable-gpu-proving") {
			err := os.Setenv("BELLMAN_NO_GPU", "true")
			if err != nil {
				return err
			}
		}

		ctx, _ := tag.New(lcli.DaemonContext(cctx),
			tag.Insert(metrics.Version, build.BuildVersion),
			tag.Insert(metrics.Commit, build.CurrentCommit),
			tag.Insert(metrics.NodeType, "miner"),
		)
		// Register all metric views
		if err := view.Register(
			metrics.MinerNodeViews...,
		); err != nil {
			log.Fatalf("Cannot register the view: %v", err)
		}
		// Set the metric to one so it is published to the exporter
		stats.Record(ctx, metrics.LotusInfo.M(1))

		nodeApi, ncloser, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			return xerrors.Errorf("getting full node api: %w", err)
		}
		defer ncloser()

		v, err := nodeApi.Version(ctx)
		if err != nil {
			return err
		}

		if cctx.Bool("manage-fdlimit") {
			if _, _, err := ulimit.ManageFdLimit(); err != nil {
				log.Errorf("setting file descriptor limit: %s", err)
			}
		}

		if v.APIVersion != api.FullAPIVersion1 {
			return xerrors.Errorf("lotus-daemon API version doesn't match: expected: %s", api.APIVersion{APIVersion: api.FullAPIVersion1})
		}

		log.Info("Checking full node sync status")

		if !cctx.Bool("nosync") {
			if err := lcli.SyncWait(ctx, &v0api.WrapperV1Full{FullNode: nodeApi}, false); err != nil {
				return xerrors.Errorf("sync wait: %w", err)
			}
		}

		// var validator address.Address
		// if cctx.Bool("default-key") {
		// 	validator, err = nodeApi.WalletDefaultAddress(ctx)
		// 	if err != nil {
		// 		return err
		// 	}
		// } else {
		// 	validator, err = address.NewFromString(cctx.Args().First())
		// 	if err != nil {
		// 		return err
		// 	}
		// }
		// if validator == address.Undef {
		// 	return xerrors.Errorf("no validator address specified as first argument for validator")
		// }

		h, err := newLp2pHost(cctx.String("repo"))
		if err != nil {
			return err
		}
		log.Info("Mir libp2p host listening in the following addresses:")
		for _, a := range h.Addrs() {
			log.Info(a)
		}

		// log.Infow("Starting mining with validator", "validator", validator)
		return nil
	},
}

const PRIVKEY = "mir.key"
const MADDR = "mir.maddr"

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, nil
}

// TODO: Consider encrypting the file and adding cmds to handle mir keys.
func lp2pID(dir string) (crypto.PrivKey, error) {
	// See if the key exists.
	path := filepath.Join(dir, PRIVKEY)
	// if it doesn't exist create a new key
	exists, err := fileExists(path)
	if !exists {
		pk, err := genLibp2pKey()
		if err != nil {
			return nil, fmt.Errorf("error generating libp2p key: %w", err)
		}
		file, err := os.Create(path)
		if err != nil {
			return nil, fmt.Errorf("error creating libp2p key: %w", err)
		}
		kbytes, err := crypto.MarshalPrivateKey(pk)
		if err != nil {
			return nil, fmt.Errorf("error marshalling libp2p key: %w", err)
		}
		_, err = file.Write(kbytes)
		if err != nil {
			return nil, fmt.Errorf("error writing libp2p key in file: %w", err)
		}
		return pk, nil
	}
	kbytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading libp2p key from file: %w", err)
	}

	// if read and return the key.
	return crypto.UnmarshalPrivateKey(kbytes)
}

func genLibp2pKey() (crypto.PrivKey, error) {
	pk, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}
	return pk, nil
}

// TODO: Should we make multiaddrs configurable?
func newLp2pHost(dir string) (host.Host, error) {
	pk, err := lp2pID(dir)
	if err != nil {
		return nil, err
	}
	// See if mutiaddr exists.
	path := filepath.Join(dir, MADDR)
	// if it doesn't exist create a new key
	exists, err := fileExists(path)
	if !exists {
		// use any free endpoints in the host.
		h, err := libp2p.New(
			libp2p.Identity(pk),
			libp2p.DefaultTransports,
			libp2p.ListenAddrStrings(
				"/ip4/0.0.0.0/tcp/0",
				"/ip6/::/tcp/0",
				"/ip4/0.0.0.0/udp/0/quic",
				"/ip6/::/udp/0/quic"),
		)
		if err != nil {
			return nil, err
		}
		file, err := os.Create(path)
		if err != nil {
			return nil, fmt.Errorf("error creating libp2p multiaddr: %w", err)
		}
		b, err := marshalMultiAddrSlice(h.Addrs())
		if err != nil {
			return nil, err
		}
		_, err = file.Write(b)
		if err != nil {
			return nil, fmt.Errorf("error writing libp2p multiaddr in file: %w", err)
		}
		return h, nil
	}
	bMaddr, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading multiaddr from file: %w", err)
	}

	addrs, err := unmarshalMultiAddrSlice(bMaddr)
	if err != nil {
		return nil, err
	}
	return libp2p.New(
		libp2p.Identity(pk),
		libp2p.DefaultTransports,
		libp2p.ListenAddrs(addrs...),
	)
}

func marshalMultiAddrSlice(ma []multiaddr.Multiaddr) ([]byte, error) {
	out := []string{}
	for _, a := range ma {
		out = append(out, a.String())
	}
	return json.Marshal(&out)
}

func unmarshalMultiAddrSlice(b []byte) ([]multiaddr.Multiaddr, error) {
	out := []multiaddr.Multiaddr{}
	s := []string{}
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	for _, mstr := range s {
		a, err := multiaddr.NewMultiaddr(mstr)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}
