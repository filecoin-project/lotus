package prof

import (
	"context"
	"fmt"
	"time"

	logging "github.com/ipfs/go-log/v2"
)

type profileLoggerKeyType struct{}

var profileLoggerKey = profileLoggerKeyType{}

type ProfileLogger interface {
	Done()
}

type profileLogger struct {
	log   *logging.ZapEventLogger
	start time.Time
	what  string
}

var _ ProfileLogger = (*profileLogger)(nil)

type emptyProfileLogger struct{}

var _ ProfileLogger = emptyProfileLogger{}

func WithProfileContext(ctx context.Context, name string) context.Context {
	logger := logging.Logger(name)
	return context.WithValue(ctx, profileLoggerKey, logger)
}

func GetProfileLogger(ctx context.Context, what string) ProfileLogger {
	if v := ctx.Value(profileLoggerKey); v != nil {
		return newProfileLogger(v.(*logging.ZapEventLogger), what)
	}

	return emptyProfileLogger{}
}

func (emptyProfileLogger) Done() {}

func newProfileLogger(log *logging.ZapEventLogger, what string) ProfileLogger {
	start := time.Now()
	return &profileLogger{start: start, log: log, what: what}
}

func (l *profileLogger) Done() {
	l.log.Infow(fmt.Sprintf("%s done", l.what), "took", time.Since(l.start))
}
