package delegator

import (
    "testing"
    "github.com/stretchr/testify/require"
)

func TestIsEip7702Code(t *testing.T) {
    var a [20]byte
    for i := range a { a[i] = 0xAA }
    code := BuildEip7702Code(a)
    require.Len(t, code, 23)
    require.True(t, IsEip7702Code(code))

    // Wrong length
    require.False(t, IsEip7702Code(code[:10]))
    // Wrong prefix
    bad := append([]byte{0xEF, 0x00, 0x00}, a[:]...)
    require.False(t, IsEip7702Code(bad))
}

