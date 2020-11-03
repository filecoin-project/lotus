package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"text/scanner"

	"github.com/chzyer/readline"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/node/repo"
)

var rpcCmd = &cli.Command{
	Name:  "rpc",
	Usage: "Interactive JsonPRC shell",
	// TODO: flag for miner/worker
	Action: func(cctx *cli.Context) error {
		addr, headers, err := lcli.GetRawAPI(cctx, repo.FullNode)
		if err != nil {
			return err
		}

		u, err := url.Parse(addr)
		if err != nil {
			return xerrors.Errorf("parsing api URL: %w", err)
		}

		switch u.Scheme {
		case "ws":
			u.Scheme = "http"
		case "wss":
			u.Scheme = "https"
		}

		addr = u.String()

		ctx := lcli.ReqContext(cctx)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		afmt := lcli.NewAppFmt(cctx.App)

		cs := readline.NewCancelableStdin(afmt.Stdin)
		go func() {
			<-ctx.Done()
			cs.Close() // nolint:errcheck
		}()

		cctx.App.Metadata["repoType"] = repo.FullNode
		if err := lcli.VersionCmd.Action(cctx); err != nil {
			return err
		}
		fmt.Println("Usage: > Method [Param1, Param2, ...]")

		rl, err := readline.NewEx(&readline.Config{
			Stdin:             cs,
			HistoryFile:       "/tmp/lotusrpc.tmp",
			Prompt:            "> ",
			EOFPrompt:         "exit",
			HistorySearchFold: true,

			// TODO: Some basic auto completion
		})
		if err != nil {
			return err
		}

		for {
			line, err := rl.Readline()
			if err == readline.ErrInterrupt {
				if len(line) == 0 {
					break
				} else {
					continue
				}
			} else if err == io.EOF {
				break
			}

			var s scanner.Scanner
			s.Init(strings.NewReader(line))
			s.Scan()
			method := s.TokenText()

			s.Scan()
			params := line[s.Position.Offset:]

			jreq, err := json.Marshal(struct {
				Jsonrpc string          `json:"jsonrpc"`
				ID      int             `json:"id"`
				Method  string          `json:"method"`
				Params  json.RawMessage `json:"params"`
			}{
				Jsonrpc: "2.0",
				Method:  "Filecoin." + method,
				Params:  json.RawMessage(params),
				ID:      0,
			})
			if err != nil {
				return err
			}

			req, err := http.NewRequest("POST", addr, bytes.NewReader(jreq))
			if err != nil {
				return err
			}
			req.Header = headers
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}

			rb, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			fmt.Println(string(rb))

			if err := resp.Body.Close(); err != nil {
				return err
			}
		}

		return nil
	},
}
