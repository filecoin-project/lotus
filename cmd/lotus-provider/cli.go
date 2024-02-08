package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/lotus/api"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/deps"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/rpc"
	"github.com/gbrlsnchs/jwt/v3"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
	"net"
	"os"
	"time"
)

const providerEnvVar = "PROVIDER_API_INFO"

var cliCmd = &cli.Command{
	Name:  "cli",
	Usage: "Execute cli commands",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "machine",
			Usage: "machine host:port",
		},
	},
	Before: func(cctx *cli.Context) error {
		if os.Getenv(providerEnvVar) != "" {
			// set already
			return nil
		}

		db, err := deps.MakeDB(cctx)
		if err != nil {
			return err
		}

		ctx := lcli.ReqContext(cctx)

		machine := cctx.String("machine")
		if machine == "" {
			// interactive picker
			var machines []struct {
				HostAndPort string    `db:"host_and_port"`
				LastContact time.Time `db:"last_contact"`
			}

			err := db.Select(ctx, &machines, "select host_and_port, last_contact from harmony_machines")
			if err != nil {
				return xerrors.Errorf("getting machine list: %w", err)
			}

			now := time.Now()
			fmt.Println("Available machines:")
			for i, m := range machines {
				// A machine is healthy if contacted not longer than 2 minutes ago
				healthStatus := "unhealthy"
				if now.Sub(m.LastContact) <= 2*time.Minute {
					healthStatus = "healthy"
				}
				fmt.Printf("%d. %s %s\n", i+1, m.HostAndPort, healthStatus)
			}

			fmt.Print("Select: ")
			reader := bufio.NewReader(os.Stdin)
			input, err := reader.ReadString('\n')
			if err != nil {
				return xerrors.Errorf("reading selection: %w", err)
			}

			var selection int
			_, err = fmt.Sscanf(input, "%d", &selection)
			if err != nil {
				return xerrors.Errorf("parsing selection: %w", err)
			}

			if selection < 1 || selection > len(machines) {
				return xerrors.New("invalid selection")
			}

			machine = machines[selection-1].HostAndPort
		}

		var apiKeys []string
		{
			var dbconfigs []struct {
				Config string `db:"config"`
				Title  string `db:"title"`
			}

			err := db.Select(ctx, &dbconfigs, "select config from harmony_config")
			if err != nil {
				return xerrors.Errorf("getting configs: %w", err)
			}

			var seen = make(map[string]struct{})

			for _, config := range dbconfigs {
				var layer struct {
					Apis struct {
						StorageRPCSecret string
					}
				}

				if _, err := toml.Decode(config.Config, &layer); err != nil {
					return xerrors.Errorf("decode config layer %s: %w", config.Title, err)
				}

				if layer.Apis.StorageRPCSecret != "" {
					if _, ok := seen[layer.Apis.StorageRPCSecret]; ok {
						continue
					}
					seen[layer.Apis.StorageRPCSecret] = struct{}{}
					apiKeys = append(apiKeys, layer.Apis.StorageRPCSecret)
				}
			}
		}

		if len(apiKeys) == 0 {
			return xerrors.New("no api keys found in the database")
		}
		if len(apiKeys) > 1 {
			return xerrors.Errorf("multiple api keys found in the database, not supported yet")
		}

		var apiToken []byte
		{
			type jwtPayload struct {
				Allow []auth.Permission
			}

			p := jwtPayload{
				Allow: api.AllPermissions,
			}

			sk, err := base64.StdEncoding.DecodeString(apiKeys[0])
			if err != nil {
				return xerrors.Errorf("decode secret: %w", err)
			}

			apiToken, err = jwt.Sign(&p, jwt.NewHS256(sk))
			if err != nil {
				return xerrors.Errorf("signing token: %w", err)
			}
		}

		{

			laddr, err := net.ResolveTCPAddr("tcp", machine)
			if err != nil {
				return xerrors.Errorf("net resolve: %w", err)
			}

			if len(laddr.IP) == 0 {
				// set localhost
				laddr.IP = net.IPv4(127, 0, 0, 1)
			}

			ma, err := manet.FromNetAddr(laddr)
			if err != nil {
				return xerrors.Errorf("net from addr (%v): %w", laddr, err)
			}

			token := fmt.Sprintf("%s:%s", string(apiToken), ma)
			if err := os.Setenv(providerEnvVar, token); err != nil {
				return xerrors.Errorf("setting env var: %w", err)
			}
		}

		{
			api, closer, err := rpc.GetProviderAPI(cctx)
			if err != nil {
				return err
			}
			defer closer()

			v, err := api.Version(ctx)
			if err != nil {
				return xerrors.Errorf("querying version: %w", err)
			}

			fmt.Println("remote node version: ", v.String())
		}

		return nil
	},
	Subcommands: []*cli.Command{},
}
