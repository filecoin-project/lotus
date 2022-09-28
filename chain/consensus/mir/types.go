package mir

import (
	logging "github.com/ipfs/go-log/v2"
)

const (
	// ConfigOffset is the number of epochs by which to delay configuration changes.
	// If a configuration is agreed upon in epoch e, it will take effect in epoch e + 1 + configOffset.
	ConfigOffset        = 2
	TransportType       = 0
	ReconfigurationType = 1
)

var log = logging.Logger("mir-consensus")
