package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	ffi "github.com/filecoin-project/filecoin-ffi"
	pb "github.com/filecoin-project/lotus/cmd/crand/pb"
	"github.com/filecoin-project/lotus/lib/lotuslog"
	lru "github.com/hashicorp/golang-lru"
	logging "github.com/ipfs/go-log/v2"
	cli "github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
)

var log = logging.Logger("crand")

var serve = &cli.Command{
	Name:        "serve",
	Description: "runs crand server",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "listen",
			Value: ":17000",
		},
		&cli.StringFlag{
			Name:  "params",
			Value: "params.json",
		},
		&cli.IntFlag{
			Name:  "cache-size",
			Value: 10 << 10,
		},
	},

	Action: func(cctx *cli.Context) error {
		if _, err := os.Stat(cctx.String("params")); os.IsNotExist(err) {
			if err := createNewParams(cctx.String("params")); err != nil {
				return err
			}
		} else if err != nil {
			return xerrors.Errorf("stat params file: %+v", err)
		}

		params, err := loadParams(cctx.String("params"))
		if err != nil {
			return err
		}
		pub := ffi.PrivateKeyPublicKey(params.Priv)
		fmt.Printf("Pubkey: %x\n", pub)
		fmt.Printf("Genesis: %s\n", params.GenesisTime)
		fmt.Printf("Round: %s\n", params.Round.D())
		cache, err := lru.NewARC(cctx.Int("cache-size"))
		if err != nil {
			return xerrors.Errorf("could not create cache with size %d : %w", cctx.Int("cache-size"), err)
		}

		list, err := net.Listen("tcp", cctx.String("listen"))
		if err != nil {
			return xerrors.Errorf("failed to listen: %v", err)
		}
		defer list.Close()
		s := grpc.NewServer()

		pb.RegisterCrandServer(s, &server{p: *params, pub: pub, cache: cache})
		fmt.Printf("Running server\n")
		return s.Serve(list)
	},
}

func setupConn(addr string, insecure bool) (pb.CrandClient, func() error, error) {
	flags := []grpc.DialOption{grpc.WithBlock()}
	if insecure {
		flags = append(flags, grpc.WithInsecure())
	}
	conn, err := grpc.Dial(addr, flags...)
	if err != nil {
		return nil, nil, xerrors.Errorf("did not connect: %+v", err)
	}
	c := pb.NewCrandClient(conn)

	return c, conn.Close, nil
}

var client = &cli.Command{
	Name:        "client",
	Description: "acceses randomness",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "addr",
			Value: "localhost:17000",
		},
		&cli.BoolFlag{
			Name:  "insecure",
			Value: false,
			Usage: "allow for insecure connection",
		},
		&cli.Uint64Flag{
			Name:  "N",
			Value: 0,
			Usage: "index of randomness to request",
		},
	},
	Subcommands: []*cli.Command{
		{
			Name:        "info",
			Description: "get crand info",
			Action: func(cctx *cli.Context) error {
				c, cls, err := setupConn(cctx.String("addr"), cctx.Bool("insecure"))
				if err != nil {
					return xerrors.Errorf("seting up connection: %w", err)
				}
				defer cls()

				ctx, cancel := context.WithTimeout(cctx.Context, time.Second)
				defer cancel()
				r, err := c.GetInfo(ctx, &pb.InfoRequest{})
				if err != nil {
					return xerrors.Errorf("get info errorr: %w", err)
				}

				fmt.Printf("Pubkey: %x\nGenesis: %s\nRound: %s\n", r.GetPubkey(),
					time.Unix(r.GetGenesisTs(), 0), time.Duration(r.GetRound()))

				return nil
			},
		},
	},
	Action: func(cctx *cli.Context) error {
		c, cls, err := setupConn(cctx.String("addr"), cctx.Bool("insecure"))
		if err != nil {
			return xerrors.Errorf("seting up connection: %w", err)
		}
		defer cls()

		ctx, cancel := context.WithTimeout(cctx.Context, time.Second)
		defer cancel()
		r, err := c.GetRandomness(ctx, &pb.RandomnessRequest{Round: cctx.Uint64("N")})
		if err != nil {
			log.Fatalf("could not get randomess: %v", err)
		}
		fmt.Printf("Randomness: %X\n", r.GetRandomness())
		return nil
	},
}

func main() {
	lotuslog.SetupLogLevels()

	app := &cli.App{
		Name:     "crand",
		Usage:    "centralized randomness",
		Version:  "v0.0.1",
		Commands: []*cli.Command{serve, client},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalf("%+v", err)
	}

}
