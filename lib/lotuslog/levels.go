package lotuslog

import (
	"log/slog"
	"os"

	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/gologshim"
)

func init() {
	// Route all slog logs through go-log for unified formatting.
	// This is needed because go-libp2p v0.44+ uses slog instead of go-log.
	slog.SetDefault(slog.New(logging.SlogHandler()))

	// Connect go-libp2p's slog bridge to go-log for per-subsystem level control.
	// This allows `lotus log set-level` to work with libp2p subsystems.
	gologshim.SetDefaultHandler(logging.SlogHandler())
}

func SetupLogLevels() {
	if _, set := os.LookupEnv("GOLOG_LOG_LEVEL"); !set {
		_ = logging.SetLogLevel("*", "INFO")
		_ = logging.SetLogLevel("dht", "ERROR")
		_ = logging.SetLogLevel("swarm2", "WARN")
		_ = logging.SetLogLevel("bitswap", "WARN")
		//_ = logging.SetLogLevel("pubsub", "WARN")
		_ = logging.SetLogLevel("connmgr", "WARN")
		_ = logging.SetLogLevel("advmgr", "DEBUG")
		_ = logging.SetLogLevel("stores", "DEBUG")
		_ = logging.SetLogLevel("nat", "INFO")
	}
	// Always mute RtRefreshManager because it breaks terminals
	_ = logging.SetLogLevel("dht/RtRefreshManager", "FATAL")
}
