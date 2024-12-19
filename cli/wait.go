package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli/v2"
)

var WaitApiCmd = &cli.Command{
	Name:  "wait-api",
	Usage: "Wait for lotus api to come online",
	Flags: []cli.Flag{
		&cli.DurationFlag{
			Name:  "timeout",
			Usage: "duration to wait till fail",
			Value: time.Second * 30,
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := ReqContext(cctx)
		ctx, cancel := context.WithTimeout(ctx, cctx.Duration("timeout"))
		defer cancel()
		for {
			if ctx.Err() != nil {
				break
			}

			api, closer, err := GetAPI(cctx)
			if err != nil {
				fmt.Printf("Not online yet... (%s)\n", err)
				time.Sleep(time.Second)
				continue
			}
			defer closer()

			_, err = api.Version(ctx)
			if err != nil {
				return err
			}

			return nil
		}

		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timed out waiting for api to come online")
		}

		return ctx.Err()
	},
}
