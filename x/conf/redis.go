package conf

import (
	redis "github.com/go-redis/redis/v7"
)

var (
	KV *redis.Client
)

func initRedis(addr, pwd string) error {
	KV = redis.NewClient(&redis.Options{
		Addr:       addr,
		Password:   pwd,
		DB:         0,
		PoolSize:   30,
		MaxRetries: 3,
	})
	return KV.Ping().Err()
}
