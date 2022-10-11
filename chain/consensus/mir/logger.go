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
	// Mir's LevelTrace is ignored.
	switch level {
	case mirlogging.LevelError:
		l.logger.Errorw(text, args...)
	case mirlogging.LevelInfo:
		l.logger.Infow(text, args...)
	case mirlogging.LevelWarn:
		l.logger.Warnw(text, args...)
	case mirlogging.LevelDebug:
		l.logger.Debugw(text, args...)
	}
}

func (l *managerLogger) MinLevel() mirlogging.LogLevel {
	level := ipfslogging.GetConfig().SubsystemLevels[managerLoggerName]
	switch level {
	case ipfslogging.LevelDebug:
		return mirlogging.LevelDebug
	case ipfslogging.LevelInfo:
		return mirlogging.LevelInfo
	case ipfslogging.LevelWarn:
		return mirlogging.LevelWarn
	case ipfslogging.LevelError:
		return mirlogging.LevelError
	case ipfslogging.LevelDPanic:
		return mirlogging.LevelError
	case ipfslogging.LevelPanic:
		return mirlogging.LevelError
	case ipfslogging.LevelFatal:
		return mirlogging.LevelError
	default:
		return mirlogging.LevelError
	}
}

func (l *managerLogger) IsConcurrent() bool {
	return true
}
