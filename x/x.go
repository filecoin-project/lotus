package x

import "github.com/filecoin-project/lotus/x/conf"

func Init(id, repo string) error {
	if err := conf.Init(id, repo); err != nil {
		return err
	}
	//todo...
	return nil
}
