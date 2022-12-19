package conf

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"hlm-ipfs/x/infras"
)

var (
	X = &Config{}
)

func Init(repo string) error {
	path := filepath.Join(repo, "lotus-x.json")
	if !infras.PathExist(path) {
		return fmt.Errorf("lotus-x.json not found: %v", path)
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, X); err != nil {
		return err
	}
	if err = initRedis(X.Redis.Addr, X.Redis.Pwd); err != nil {
		return err
	}
	return nil
}
