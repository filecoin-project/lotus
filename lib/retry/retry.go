package retry

import (
	"github.com/filecoin-project/lotus/api"
	logging "github.com/ipfs/go-log/v2"
	"time"
)

var log = logging.Logger("retry")

func Retry[T any](attempts int, initialBackoff time.Duration, errorTypes []error, f func() (T, error)) (result T, err error) {
	for i := 0; i < attempts; i++ {
		if i > 0 {
			log.Info("Retrying after error:", err)
			time.Sleep(initialBackoff)
			initialBackoff *= 2
		}
		result, err = f()
		if err == nil || !api.ErrorIsIn(err, errorTypes) {
			return result, err
		}
	}
	log.Errorf("Failed after %d attempts, last error: %s", attempts, err)
	return result, err
}
