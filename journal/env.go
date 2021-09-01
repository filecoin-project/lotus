package journal

import (
	"os"
	"strconv"
)

// envJournalDisabledEvents is the environment variable through which disabled
// journal events can be customized.
const envDisabledEvents = "LOTUS_JOURNAL_DISABLED_EVENTS"

var (
	EnvMaxBackups = envIntParser("LOTUS_JOURNAL_MAX_BACKUPS", 3)
	EnvMaxSize    = envIntParser("LOTUS_JOURNAL_MAX_SIZE", 1<<30)
)

func EnvDisabledEvents() DisabledEvents {
	if env, ok := os.LookupEnv(envDisabledEvents); ok {
		if ret, err := ParseDisabledEvents(env); err == nil {
			return ret
		}
	}
	// fallback if env variable is not set, or if it failed to parse.
	return DefaultDisabledEvents
}

func envIntParser(env string, withDefault int64) int64 {
	e, ok := os.LookupEnv(env)
	if !ok {
		return withDefault
	}
	i, err := strconv.Atoi(e)
	if err != nil {
		return withDefault
	}
	return int64(i)
}
