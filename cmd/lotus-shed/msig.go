package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	cliutil "github.com/filecoin-project/lotus/cli/util"
)

var GetFullNodeAPI = cliutil.GetFullNodeAPI
var ReqContext = cliutil.ReqContext

func LoadTipSet(ctx context.Context, cctx *cli.Context, api v0api.FullNode) (*types.TipSet, error) {
	tss := cctx.String("tipset")
	if tss == "" {
		return api.ChainHead(ctx)
	}

	return ParseTipSetRef(ctx, api, tss)
}

func ParseTipSetRef(ctx context.Context, api v0api.FullNode, tss string) (*types.TipSet, error) {
	if tss[0] == '@' {
		if tss == "@head" {
			return api.ChainHead(ctx)
		}

		var h uint64
		if _, err := fmt.Sscanf(tss, "@%d", &h); err != nil {
			return nil, xerrors.Errorf("parsing height tipset ref: %w", err)
		}

		return api.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(h), types.EmptyTSK)
	}

	cids, err := ParseTipSetString(tss)
	if err != nil {
		return nil, err
	}

	if len(cids) == 0 {
		return nil, nil
	}

	k := types.NewTipSetKey(cids...)
	ts, err := api.ChainGetTipSet(ctx, k)
	if err != nil {
		return nil, err
	}

	return ts, nil
}

func ParseTipSetString(ts string) ([]cid.Cid, error) {
	strs := strings.Split(ts, ",")

	var cids []cid.Cid
	for _, s := range strs {
		c, err := cid.Parse(strings.TrimSpace(s))
		if err != nil {
			return nil, err
		}
		cids = append(cids, c)
	}

	return cids, nil
}

type msigBriefInfo struct {
	ID        address.Address
	Signer    interface{}
	Balance   abi.TokenAmount
	Threshold float64
}

var msigCmd = &cli.Command{
	Name:  "multisig",
	Usage: "utils for multisig actors",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "specify tipset to call method on (pass comma separated array of cids)",
		},
	},
	Subcommands: []*cli.Command{
		multisigGetAllCmd,
	},
}

var multisigGetAllCmd = &cli.Command{
	Name:  "all",
	Usage: "get all multisig actor on chain with id, siigners, threshold and balance",
	Flags: []cli.Flag{
		&cli.UintFlag{
			Name:  "network-version",
			Value: uint(build.NewestNetworkVersion),
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		actors, err := api.StateListActors(ctx, ts.Key())
		if err != nil {
			return err
		}

		nv := network.Version(cctx.Uint64("network-version"))
		codeCids, err := api.StateActorCodeCIDs(ctx, nv)
		if err != nil {
			return err
		}

		msigCid, exists := codeCids["multisig"]
		if !exists {
			return xerrors.Errorf("bad code cid key")
		}

		var msigActorsInfo []msigBriefInfo
		for _, actor := range actors {

			act, err := api.StateGetActor(ctx, actor, ts.Key())
			if err != nil {
				return err
			}

			if act.Code == msigCid {

				actorState, err := api.StateReadState(ctx, actor, ts.Key())
				if err != nil {
					return err
				}

				stateI, ok := actorState.State.(map[string]interface{})
				if !ok {
					return xerrors.Errorf("fail to map msig state")
				}

				signersI, _ := stateI["Signers"]
				signers := signersI.([]interface{})
				thresholdI, _ := stateI["NumApprovalsThreshold"]
				threshold := thresholdI.(float64)
				info := msigBriefInfo{
					ID:        actor,
					Signer:    signers,
					Balance:   actorState.Balance,
					Threshold: threshold,
				}
				msigActorsInfo = append(msigActorsInfo, info)
			}
		}
		out, err := json.MarshalIndent(msigActorsInfo, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	},
}
