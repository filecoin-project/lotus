package main

import (
	"context"
	"log"
	"net/http"
	"time"

	manet "github.com/multiformats/go-multiaddr-net"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/jsonrpc"
	"github.com/filecoin-project/lotus/node/repo"
)

func getAPI(path string) (string, http.Header, error) {
	r, err := repo.NewFS(path)
	if err != nil {
		return "", nil, err
	}

	ma, err := r.APIEndpoint()
	if err != nil {
		return "", nil, xerrors.Errorf("failed to get api endpoint: %w", err)
	}
	_, addr, err := manet.DialArgs(ma)
	if err != nil {
		return "", nil, err
	}
	var headers http.Header
	token, err := r.APIToken()
	if err != nil {
		log.Printf("Couldn't load CLI token, capabilities may be limited: %w", err)
	} else {
		headers = http.Header{}
		headers.Add("Authorization", "Bearer "+string(token))
	}

	return "ws://" + addr + "/rpc/v0", headers, nil
}

func WaitForSyncComplete(ctx context.Context, napi api.FullNode) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
			head, err := napi.ChainHead(ctx)
			if err != nil {
				return err
			}

			log.Printf("Height %d", head.Height())

			if time.Now().Unix()-int64(head.MinTimestamp()) < build.BlockDelay {
				return nil
			}
		}
	}
}

func GetTips(ctx context.Context, api api.FullNode, lastHeight uint64) (<-chan *types.TipSet, error) {
	chmain := make(chan *types.TipSet)

	notif, err := api.ChainNotify(ctx)
	if err != nil {
		return nil, err
	}

	go func() {
		defer close(chmain)

		ping := time.Tick(30 * time.Second)

		for {
			select {
			case changes := <-notif:
				for _, change := range changes {
					log.Printf("Head event { height:%d; type: %s }", change.Val.Height(), change.Type)

					switch change.Type {
					case store.HCCurrent:
						tipsets, err := loadTipsets(ctx, api, change.Val, lastHeight)
						if err != nil {
							log.Print(err)
							return
						}

						for _, tipset := range tipsets {
							chmain <- tipset
						}
					case store.HCApply:
						chmain <- change.Val
					}
				}
			case <-ping:
				log.Print("Running health check")

				cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
				if _, err := api.ID(cctx); err != nil {
					log.Print("Health check failed")
					return
				}

				cancel()

				log.Print("Node online")
			case <-ctx.Done():
				return
			}
		}
	}()

	return chmain, nil
}

func loadTipsets(ctx context.Context, api api.FullNode, curr *types.TipSet, lowestHeight uint64) ([]*types.TipSet, error) {
	tipsets := []*types.TipSet{}
	for {
		if curr.Height() == 0 {
			break
		}

		if curr.Height() <= lowestHeight {
			break
		}

		log.Printf("Walking back { height:%d }", curr.Height())
		tipsets = append(tipsets, curr)

		tsk := types.NewTipSetKey(curr.Parents()...)
		prev, err := api.ChainGetTipSet(ctx, tsk)
		if err != nil {
			return tipsets, err
		}

		curr = prev
	}

	for i, j := 0, len(tipsets)-1; i < j; i, j = i+1, j-1 {
		tipsets[i], tipsets[j] = tipsets[j], tipsets[i]
	}

	return tipsets, nil
}

func GetFullNodeAPI(repo string) (api.FullNode, jsonrpc.ClientCloser, error) {
	addr, headers, err := getAPI(repo)
	if err != nil {
		return nil, nil, err
	}

	return client.NewFullNodeRPC(addr, headers)
}
