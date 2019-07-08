package crypto_test

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/go-filecoin/crypto"
	tf "github.com/filecoin-project/go-filecoin/testhelpers/testflags"
)

func TestGenerateKey(t *testing.T) {
	tf.UnitTest(t)

	rand.Seed(time.Now().UnixNano())

	sk, err := crypto.GenerateKey()
	assert.NoError(t, err)

	assert.Equal(t, len(sk), 32)

	msg := make([]byte, 32)
	for i := 0; i < len(msg); i++ {
		msg[i] = byte(i)
	}

	digest, err := crypto.Sign(sk, msg)
	assert.NoError(t, err)
	assert.Equal(t, len(digest), 65)
	pk := crypto.PublicKey(sk)

	// valid signature
	assert.True(t, crypto.Verify(pk, msg, digest))

	// invalid signature - different message (too short)
	assert.False(t, crypto.Verify(pk, msg[3:], digest))

	// invalid signature - different message
	msg2 := make([]byte, 32)
	copy(msg2, msg)
	rand.Shuffle(len(msg2), func(i, j int) { msg2[i], msg2[j] = msg2[j], msg2[i] })
	assert.False(t, crypto.Verify(pk, msg2, digest))

	// invalid signature - different digest
	digest2 := make([]byte, 65)
	copy(digest2, digest)
	rand.Shuffle(len(digest2), func(i, j int) { digest2[i], digest2[j] = digest2[j], digest2[i] })
	assert.False(t, crypto.Verify(pk, msg, digest2))

	// invalid signature - digest too short
	assert.False(t, crypto.Verify(pk, msg, digest[3:]))
	assert.False(t, crypto.Verify(pk, msg, digest[:29]))

	// invalid signature - digest too long
	digest3 := make([]byte, 70)
	copy(digest3, digest)
	assert.False(t, crypto.Verify(pk, msg, digest3))

	recovered, err := crypto.EcRecover(msg, digest)
	assert.NoError(t, err)
	assert.Equal(t, recovered, crypto.PublicKey(sk))
}
