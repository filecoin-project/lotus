package lotuslog

import (
	"os"

	logging "github.com/ipfs/go-log/v2"
)

//nolint:gosec
func SetupLogLevels() {
	if _, set := os.LookupEnv("GOLOG_LOG_LEVEL"); !set {
		logging.SetLogLevel("*", "INFO")
		logging.SetLogLevel("dht", "ERROR")
		logging.SetLogLevel("swarm2", "WARN")
		logging.SetLogLevel("bitswap", "WARN")
		//logging.SetLogLevel("pubsub", "WARN")
		logging.SetLogLevel("connmgr", "WARN")
		logging.SetLogLevel("advmgr", "DEBUG")
		logging.SetLogLevel("stores", "DEBUG")
		logging.SetLogLevel("nat", "INFO")
	}
}
