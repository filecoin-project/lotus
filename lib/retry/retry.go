package retry

import (
	"errors"
	"reflect"
	"time"

	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("retry")

func ErrorIsIn(err error, errorTypes []error) bool {
	for _, etype := range errorTypes {
		tmp := reflect.New(reflect.PointerTo(reflect.ValueOf(etype).Elem().Type())).Interface()
		if errors.As(err, tmp) {
			return true
		}
	}
	return false
}

func Retry[T any](attempts int, initialBackoff time.Duration, errorTypes []error, f func() (T, error)) (result T, err error) {
	for i := 0; i < attempts; i++ {
		if i > 0 {
			log.Info("Retrying after error:", err)
			time.Sleep(initialBackoff)
			initialBackoff *= 2
		}
		result, err = f()
		if err == nil || !ErrorIsIn(err, errorTypes) {
			return result, err
		}
	}
	log.Errorf("Failed after %d attempts, last error: %s", attempts, err)
	return result, err
}
