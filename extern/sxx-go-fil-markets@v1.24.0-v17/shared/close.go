package shared

import (
	"context"
	"errors"
	"time"
)

// When we close the data transfer, we also send a cancel message to the peer.
// CloseDataTransferTimeout is the amount of time to wait for the close to
// complete before giving up.
const CloseDataTransferTimeout = 30 * time.Second

func IsCtxDone(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
