package micro

import (
	"fmt"
	"time"

	"hlm-ipfs/x/logger"
)

const (
	MinerName  = "lotus.miner"
	WorkerName = "lotus.worker"
)

var (
	interval = time.Second * 5
	timeout  = time.Second * 10
)

func Register(serviceName, instanceName, endpoint string) error {
	if err := cache.Set(serviceName, instanceName, endpoint); err != nil {
		return err
	}
	go func() {
		for {
			<-time.After(interval)
			if err := cache.Set(serviceName, instanceName, endpoint); err != nil {
				logger.Errorw("", "err", err)
			}
		}
	}()
	return nil
}

func Select(serviceName, instanceName string) (string, error) {
	endpoint, expire, err := cache.Get(serviceName, instanceName)
	if err != nil {
		return "", err
	}
	if time.Since(expire) > timeout {
		return "", fmt.Errorf("service(%v@%v) down", serviceName, instanceName)
	}
	return endpoint, nil
}

func Selects(serviceName string) (map[string]string, error) {
	dict, err := cache.Gets(serviceName)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for instance, info := range dict {
		if time.Since(info.Heartbeat) <= timeout {
			out[instance] = info.Endpoint
		}
	}
	return out, nil
}
