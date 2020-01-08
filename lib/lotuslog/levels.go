package lotuslog

import logging "github.com/ipfs/go-log"

func SetupLogLevels() {
	logging.SetLogLevel("*", "INFO")
	logging.SetLogLevel("dht", "ERROR")
	logging.SetLogLevel("swarm2", "WARN")
	logging.SetLogLevel("bitswap", "WARN")
	logging.SetLogLevel("pubsub", "WARN")
	logging.SetLogLevel("connmgr", "WARN")
}
