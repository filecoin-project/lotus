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

const (
	ConfigMessageType = 0 // Mir specific config message
	SignedMessageType = 1 // Lotus signed message
)

func NewSignedMessageBytes(msg, opaque []byte) []byte {
	var payload []byte
	payload = append(msg, opaque...)
	payload = append(payload, SignedMessageType)
	return payload
}

func NewConfigMessageBytes(msg, opaque []byte) []byte {
	var payload []byte
	payload = append(msg, opaque...)
	payload = append(payload, ConfigMessageType)
	return payload
}
