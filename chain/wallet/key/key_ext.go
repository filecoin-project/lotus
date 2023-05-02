package key

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"io"

	"golang.org/x/xerrors"
)

var AddrPrefix = []byte{0xff, 0xff, 0xff, 0xff, 0xff} // addrPrefix = "/////"
var WalletPasswd string = ""
var PasswdPath string = ""

// wallet-security AESEncrypt/AESDecrypt
// IsSetup check setup password for wallet
func IsSetup() bool {
	return PasswdPath != ""
}

// IsLock check setup lock for wallet
func IsLock() bool {
	return WalletPasswd == ""
}

func AESEncrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, xerrors.Errorf("passwd must 6 to 16 characters")
	}

	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], plaintext)

	return ciphertext, nil
}

func AESDecrypt(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, xerrors.Errorf("passwd must 6 to 16 characters")
	} else if len(ciphertext) < aes.BlockSize {
		return nil, xerrors.Errorf("passwd must 6 to 16 characters")
	}

	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]
	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(ciphertext, ciphertext)

	return ciphertext, nil
}

func UnMakeByte(pk []byte) ([]byte, error) {
	if !IsSetup() {
		return pk, nil
	}

	if !bytes.Equal(pk[:4], AddrPrefix) {
		return pk, nil
	} else if !IsLock() {
		msg := make([]byte, len(pk)-4)
		copy(msg, pk[4:])
		m5_passwd, _ := hex.DecodeString(WalletPasswd)
		return AESDecrypt(m5_passwd, msg)
	}
	return nil, xerrors.Errorf("wallet is lock")
}

func MakeByte(pk []byte) ([]byte, error) {
	if !IsSetup() {
		return pk, nil
	}

	if IsLock() {
		return nil, xerrors.Errorf("wallet is lock")
	}

	m5_passwd, _ := hex.DecodeString(WalletPasswd)
	msg, err := AESEncrypt(m5_passwd, pk)
	if err != nil {
		return nil, err
	}
	text := make([]byte, len(msg)+4)
	copy(text[:4], AddrPrefix)
	copy(text[4:], msg)
	return text, nil
}

func IsPrivateKeyEnc(pk []byte) bool {
	if !IsSetup() || !bytes.Equal(pk[:4], AddrPrefix) {
		return false
	}
	return true
}
