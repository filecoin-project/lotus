package micro

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/filecoin-project/lotus/x/conf"
)

const (
	prefix = "lotus:x"
)

var (
	cache = &microCache{
		prefix: fmt.Sprintf("%v:%v", prefix, "micro"),
	}
)

type MicroItem struct {
	Instance  string
	Endpoint  string
	Heartbeat time.Time
}

type microCache struct {
	prefix string
}

func (c *microCache) key(serviceName string) string {
	return fmt.Sprintf("%v:%v", c.prefix, serviceName)
}

func (c *microCache) toValue(endpoint string) string {
	return fmt.Sprintf("%v@%v", endpoint, time.Now().Unix())
}

func (c *microCache) fromValue(str string) (string, time.Time, error) {
	var (
		err      error
		endpoint = ""
		expire   int64
	)
	arr := strings.Split(str, "@")
	if len(arr) < 2 {
		return "", time.Time{}, errors.New("value invalid")
	} else {
		endpoint = strings.TrimSpace(arr[0])
	}
	if expire, err = strconv.ParseInt(arr[1], 10, 64); err != nil {
		return "", time.Time{}, err
	}
	return endpoint, time.Unix(expire, 0), nil
}

func (c *microCache) Set(serviceName, instanceName, endpoint string) error {
	return conf.KV.HSet(c.key(serviceName), instanceName, c.toValue(endpoint)).Err()
}

func (c *microCache) Get(serviceName, instanceName string) (string, time.Time, error) {
	str, err := conf.KV.HGet(c.key(serviceName), instanceName).Result()
	if err != nil {
		return "", time.Time{}, err
	}
	return c.fromValue(str)
}

func (c *microCache) Gets(serviceName string) (map[string]*MicroItem, error) {
	dict, err := conf.KV.HGetAll(c.key(serviceName)).Result()
	if err != nil {
		return nil, err
	}
	out := map[string]*MicroItem{}
	for instance, value := range dict {
		if endpoint, expire, err := c.fromValue(value); err == nil {
			out[instance] = &MicroItem{Endpoint: endpoint, Heartbeat: expire}
		}
	}
	return out, nil
}
