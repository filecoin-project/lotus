package main

import (
	"context"
	"log"
	"net/http"

	"github.com/multiformats/go-multiaddr-net"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/api/client"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/lib/jsonrpc"
	"github.com/filecoin-project/go-lotus/node/repo"
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

func GetTips(ctx context.Context, api api.FullNode, lastHeight uint64) (<-chan *types.TipSet, error) {
	chmain := make(chan *types.TipSet)

	notif, err := api.ChainNotify(ctx)
	if err != nil {
		return nil, err
	}

	go func() {
		defer close(chmain)

		for {
			select {
			case changes := <-notif:
				for _, change := range changes {
					log.Printf("Head event { height:%d; type: %s }", change.Val.Height(), change.Type)

					switch change.Type {
					case store.HCCurrent:
						tipsets := []*types.TipSet{}
						curr := change.Val

						for {
							if curr.Height() == 0 {
								break
							}

							if curr.Height() <= lastHeight {
								break
							}

							log.Printf("Walking back { height:%d }", curr.Height())
							tipsets = append(tipsets, curr)

							ph := ParentTipsetHeight(curr)
							if ph == 0 {
								break
							}

							prev, err := api.ChainGetTipSetByHeight(ctx, ph, curr)
							if err != nil {
								log.Fatal(err)
								return
							}

							curr = prev
						}

						for i, j := 0, len(tipsets)-1; i < j; i, j = i+1, j-1 {
							tipsets[i], tipsets[j] = tipsets[j], tipsets[i]
						}

						for _, tipset := range tipsets {
							chmain <- tipset
						}
					case store.HCApply:
						chmain <- change.Val
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return chmain, nil
}

func ParentTipsetHeight(tipset *types.TipSet) uint64 {
	mtb := tipset.MinTicketBlock()
	return tipset.Height() - uint64(len(mtb.Tickets)) - 1
}

func GetFullNodeAPI(repo string) (api.FullNode, jsonrpc.ClientCloser, error) {
	addr, headers, err := getAPI(repo)
	if err != nil {
		return nil, nil, err
	}

	return client.NewFullNodeRPC(addr, headers)
}
