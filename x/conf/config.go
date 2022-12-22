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
	path := filepath.Join(repo, "config.json")
	if !infras.PathExist(path) {
		return fmt.Errorf("config.json not found: %v", path)
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, X); err != nil {
		return err
	}
	return initRedis(X.Redis.Addr, X.Redis.Pwd)
}
