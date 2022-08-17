package retry

import (
	"time"

	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("retry")

func Retry[T any](attempts int, sleep int, f func() (T, error)) (result T, err error) {
	for i := 0; i < attempts; i++ {
		if i > 0 {
			log.Info("Retrying after error:", err)
			time.Sleep(time.Duration(sleep) * time.Second)
			sleep *= 2
		}
		result, err = f()
		if err == nil {
			return result, nil
		}
	}
	log.Errorf("Failed after %d attempts, last error: %s", attempts, err)
	return result, err
}
