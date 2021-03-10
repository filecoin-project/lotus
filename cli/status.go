package cli

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

var statusCmd = &cli.Command{
	Name:  "status",
	Usage: "Check node status",
	Action: func(cctx *cli.Context) error {
		apic, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		status, err := apic.NodeStatus(ctx)
		if err != nil {
			return err
		}

		fmt.Println("node status:")
		fmt.Printf("Sync Epoch: %d\n", status.SyncStatus.Epoch)
		fmt.Printf("Epochs Behind: %d\n", status.SyncStatus.Behind)
		fmt.Printf("Peers to Publish Messages: %d\n", status.PeerStatus.PeersToPublishMsgs)
		fmt.Printf("Peers to Publish Blocks: %d\n", status.PeerStatus.PeersToPublishBlocks)

		return nil
	},
}
