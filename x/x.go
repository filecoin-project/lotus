package x

import "github.com/filecoin-project/lotus/x/conf"

func Init(repo string) error {
	if err := conf.Init(repo); err != nil {
		return err
	}
	//todo...
	return nil
}
