package filbase_redis

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/samber/lo"
)

var ctx = context.Background()
var rdb redis.Client = *redis.NewClient(lo.Must(redis.ParseURL(os.Getenv("REDIS_URL"))))
var prefix string = os.Getenv("MINER_REDIS_PREFIX")

// 程序启动时运行，确保 Redis 正确配置
// <prefix>:validate:<uuid> string
func ValidateRedis() {
	_, err := rdb.Set(ctx, prefix+":validate:"+uuid.New().String(), "validated", 1).Result()
	if err != nil {
		fmt.Printf("redis asscess failed")
		panic(err.Error())
	}
}

// 记录 Sector 对 Worker 的分配关系
// hash: <prefix>:sector-assignment:<worker-type> {sectorId:workerHostName}
type WorkerKey string

const (
	CWorkerKey WorkerKey = "c-worker"
	PWorkerKey WorkerKey = "p-worker"
)

func SetWorkerForSector(workerType WorkerKey, sectorId int, workerHostName string) (int64, error) {
	return rdb.HSet(ctx, prefix+":sector-assignment:"+string(workerType), strconv.Itoa(sectorId), workerHostName).Result()
}

func GetWorkerForSector(workerType WorkerKey, sectorId int) (string, error) {
	return rdb.HGet(ctx, prefix+":sector-assignment:"+string(workerType), strconv.Itoa(sectorId)).Result()
}

func GetAllWorkerSector(workerType WorkerKey) ([]interface{}, error) {
	return rdb.HMGet(ctx, prefix+":sector-assignment:"+string(workerType)).Result()
}
