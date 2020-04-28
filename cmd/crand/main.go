package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"time"

	ffi "github.com/filecoin-project/filecoin-ffi"
	pb "github.com/filecoin-project/lotus/cmd/crand/pb"
	"github.com/filecoin-project/lotus/lib/lotuslog"
	logging "github.com/ipfs/go-log/v2"
	cli "github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var log = logging.Logger("crand")

type server struct {
	p Params
	pb.UnimplementedCrandServer
}

func (s *server) GetRandomness(_ context.Context, rq *pb.RandomnessRequest) (*pb.RandomnessReply, error) {
	rnd := rq.Round
	if time.Since(s.p.GenesisTime.Add(s.p.Round.D()*time.Duration(rnd))) < 0 {
		return nil, status.Errorf(codes.Unavailable, "randomenss is part of the future")
	}

	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, rq.Round)

	sig := ffi.PrivateKeySign(s.p.Priv, buf)
	return &pb.RandomnessReply{Randomness: sig[:]}, nil
}

type JDuration time.Duration

func (d *JDuration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = JDuration(dur)
	return nil
}

func (d JDuration) MarshalJSON() ([]byte, error) {
	return []byte("\"" + time.Duration(d).String() + "\""), nil
}
func (d JDuration) D() time.Duration {
	return time.Duration(d)
}

type Params struct {
	Priv        ffi.PrivateKey
	GenesisTime time.Time
	Round       JDuration
}

func createNewParams(fname string) error {
	defParams := Params{
		GenesisTime: time.Now().UTC().Round(1 * time.Second),
		Round:       JDuration(30 * time.Second),
	}
	pk := ffi.PrivateKeyGenerate()
	defParams.Priv = pk
	params, err := json.Marshal(defParams)
	if err != nil {
		return xerrors.Errorf("marshaling params: %w", err)
	}

	err = ioutil.WriteFile(fname, params, 0600)
	if err != nil {
		return xerrors.Errorf("writing file: %w", err)
	}
	return nil
}

func loadParams(fname string) (*Params, error) {
	b, err := ioutil.ReadFile(fname)
	if err != nil {
		return nil, xerrors.Errorf("reading file: %w", err)
	}
	var p Params
	err = json.Unmarshal(b, &p)
	if err != nil {
		return nil, xerrors.Errorf("unmarshal: %w", err)
	}

	return &p, nil
}

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
		fmt.Printf("pubkey: %x\n", pub)
		fmt.Printf("genesis: %s\n", params.GenesisTime)
		fmt.Printf("round: %s\n", params.Round.D())

		list, err := net.Listen("tcp", cctx.String("listen"))
		if err != nil {
			xerrors.Errorf("failed to listen: %v", err)
		}
		s := grpc.NewServer()
		pb.RegisterCrandServer(s, &server{p: *params})
		fmt.Printf("Runing server\n")
		return s.Serve(list)
	},
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
	Action: func(cctx *cli.Context) error {
		flags := []grpc.DialOption{grpc.WithBlock()}
		if cctx.Bool("insecure") {
			flags = append(flags, grpc.WithInsecure())
		}
		conn, err := grpc.Dial(cctx.String("addr"), flags...)
		if err != nil {
			log.Fatalf("did not connect: %v", err)
		}
		defer conn.Close()
		c := pb.NewCrandClient(conn)

		ctx, cancel := context.WithTimeout(cctx.Context, time.Second)
		defer cancel()
		r, err := c.GetRandomness(ctx, &pb.RandomnessRequest{Round: cctx.Uint64("N")})
		if err != nil {
			log.Fatalf("could not get randomess: %v", err)
		}
		fmt.Printf("Randomness: %X", r.GetRandomness())
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
