package mir

import (
	ipfslogging "github.com/ipfs/go-log/v2"

	mirlogging "github.com/filecoin-project/mir/pkg/logging"
)

const managerLoggerName = "mir-manager"

var _ mirlogging.Logger = &managerLogger{}

// mirLogger implements Mir's Log interface.
type managerLogger struct {
	logger *ipfslogging.ZapEventLogger
}

func newManagerLogger() *managerLogger {
	return &managerLogger{
		logger: ipfslogging.Logger(managerLoggerName),
	}
}

// Log logs a message with additional context.
func (l *managerLogger) Log(level mirlogging.LogLevel, text string, args ...interface{}) {
	switch level {
	case mirlogging.LevelError:
		l.logger.Errorw(text, "error", args)
	case mirlogging.LevelInfo:
		l.logger.Infow(text, "info", args)
	case mirlogging.LevelWarn:
		l.logger.Warnw(text, "warn", args)
	case mirlogging.LevelDebug:
		l.logger.Debugw(text, "debug", args)
	}
}

func (l *managerLogger) MinLevel() mirlogging.LogLevel {
	return mirlogging.LevelDisable
}

func (l *managerLogger) IsConcurrent() bool {
	return true
}
