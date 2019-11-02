package message_test

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTransferMessage(t *testing.T) {
}

func TestTransferMessage_UnmarshalCBOR(t *testing.T) {
	treq := NewTestTransferRequest()
	assert.NotNil(t, treq)
}
