package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"time"

	"github.com/chzyer/readline"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/itests/kit"
)

var itestdCmd = &cli.Command{
	Name:        "itestd",
	Description: "Integration test debug env",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "listen",
			Value: "127.0.0.1:5674",
		},
		&cli.StringFlag{
			Name:  "http-server-timeout",
			Value: "30s",
		},
	},
	Action: func(cctx *cli.Context) error {
		var nodes []kit.ItestdNotif

		m := http.NewServeMux()
		m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			var notif kit.ItestdNotif
			if err := json.NewDecoder(r.Body).Decode(&notif); err != nil {
				fmt.Printf("!! Decode itest notif: %s\n", err)
				return
			}

			fmt.Printf("%d @%s '%s=%s'\n", len(nodes), notif.TestName, notif.NodeType, notif.Api)
			nodes = append(nodes, notif)
		})
		l, err := net.Listen("tcp", cctx.String("listen"))
		if err != nil {
			return xerrors.Errorf("net listen: %w", err)
		}

		timeout, err := time.ParseDuration(cctx.String("http-server-timeout"))
		if err != nil {
			return xerrors.Errorf("invalid time string %s: %x", cctx.String("http-server-timeout"), err)
		}
		s := &httptest.Server{
			Listener: l,
			Config:   &http.Server{Handler: m, ReadHeaderTimeout: timeout},
		}
		s.Start()
		fmt.Printf("ITest env:\n\nLOTUS_ITESTD=%s\n\nSay 'sh' to spawn a shell connected to test nodes\n--- waiting for clients\n", s.URL)

		cs := readline.NewCancelableStdin(os.Stdin)
		go func() {
			<-cctx.Done()
			cs.Close() // nolint:errcheck
		}()

		rl := bufio.NewReader(cs)

		for {
			cmd, _, err := rl.ReadLine()
			if err != nil {
				return xerrors.Errorf("readline: %w", err)
			}

			switch string(cmd) {
			case "sh":
				shell := "/bin/sh"
				if os.Getenv("SHELL") != "" {
					shell = os.Getenv("SHELL")
				}

				p := exec.Command(shell, "-i")
				p.Env = append(p.Env, os.Environ()...)
				lastNodes := map[string]string{}
				for _, node := range nodes {
					lastNodes[node.NodeType] = node.Api
				}
				if _, found := lastNodes["MARKETS_API_INFO"]; !found {
					lastNodes["MARKETS_API_INFO"] = lastNodes["MINER_API_INFO"]
				}
				for typ, api := range lastNodes {
					p.Env = append(p.Env, fmt.Sprintf("%s=%s", typ, api))
				}

				p.Stdout = os.Stdout
				p.Stderr = os.Stderr
				p.Stdin = os.Stdin
				if err := p.Start(); err != nil {
					return xerrors.Errorf("start shell: %w", err)
				}
				if err := p.Wait(); err != nil {
					fmt.Printf("wait for shell: %s\n", err)
				}
				fmt.Println("\n--- shell quit")

			default:
				fmt.Println("!! Unknown command")
			}
		}
	},
}
