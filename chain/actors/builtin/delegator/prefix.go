package delegator

// IsEip7702Code returns true if the provided bytecode is an EIP-7702
// delegation indicator of the form 0xef 0x01 0x00 || <20-byte address>.
func IsEip7702Code(code []byte) bool {
    if len(code) != 23 {
        return false
    }
    return code[0] == 0xEF && code[1] == 0x01 && code[2] == 0x00
}

// BuildEip7702Code returns the 23-byte delegation indicator code for the
// provided 20-byte delegate address.
func BuildEip7702Code(delegate [20]byte) []byte {
    out := make([]byte, 23)
    out[0] = 0xEF
    out[1] = 0x01
    out[2] = 0x00
    copy(out[3:], delegate[:])
    return out
}

