package worker

import (
	"fmt"
	"os"
	"sort"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var resourcesCmd = &cli.Command{
	Name:  "resources",
	Usage: "Manage resource table overrides",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "all",
			Usage: "print all resource envvars",
		},
		&cli.BoolFlag{
			Name:  "default",
			Usage: "print default resource envvars",
		},
	},
	Action: func(cctx *cli.Context) error {
		def := map[string]string{}
		set := map[string]string{}
		all := map[string]string{}

		_, err := storiface.ParseResourceEnv(func(key, d string) (string, bool) {
			if d != "" {
				all[key] = d
				def[key] = d
			}

			s, ok := os.LookupEnv(key)
			if ok {
				all[key] = s
				set[key] = s
			}

			return s, ok
		})
		if err != nil {
			return err
		}

		printMap := func(m map[string]string) {
			var arr []string
			for k, v := range m {
				arr = append(arr, fmt.Sprintf("%s=%s", k, v))
			}
			sort.Strings(arr)
			for _, s := range arr {
				fmt.Println(s)
			}
		}

		if cctx.Bool("default") {
			printMap(def)
		} else {
			if cctx.Bool("all") {
				printMap(all)
			} else {
				printMap(set)
			}
		}

		return nil
	},
}
