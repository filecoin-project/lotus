package wallet

import (
	/* #nosec G501 */
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
)

var substitutePwd = []byte("****************")

const walletSaltPwd string = "8XBMT5OruN6XwEhaYfJfMJPL3WUhjWmH"
const walletCheckMsg string = "check passwd is success"

type KeyInfo struct {
	types.KeyInfo
	Enc bool
}

func completionPwd(pwd []byte) []byte {
	sub := 16 - len(pwd)
	if sub > 0 {
		pwd = append(pwd, substitutePwd[:sub]...)
	}
	return pwd
}

func SetupPasswd(passwd []byte, path string) error {
	_, err := os.Stat(path)
	if err == nil {
		return xerrors.Errorf("checking file before Setup passwd '%s': file already exists", path)
	} else if !os.IsNotExist(err) {
		return xerrors.Errorf("checking file before Setup passwd '%s': %w", path, err)
	}

	//用户密码加密check消息
	passwd = completionPwd(passwd)
	/* #nosec G401 */
	m5 := md5.Sum(passwd)
	checkmsg, err := key.AESEncrypt(m5[:], []byte(walletCheckMsg))
	m5passwd := hex.EncodeToString(m5[:])
	if err != nil {
		return err
	}

	//用户密码用盐密加密
	saltkey := completionPwd([]byte(walletSaltPwd))
	/* #nosec G401 */
	saltm5 := md5.Sum(saltkey)
	saltm5passwdmsg, err := key.AESEncrypt(saltm5[:], []byte(m5passwd))
	//saltm5passwd := hex.EncodeToString(saltm5[:])
	if err != nil {
		return err
	}

	//存到/passwd
	savetext := make([]byte, 64+len(checkmsg))
	copy(savetext[:64], saltm5passwdmsg)
	copy(savetext[64:], checkmsg)
	err = ioutil.WriteFile(path, savetext, 0600)
	if err != nil {
		return xerrors.Errorf("writing file '%s': %w", path, err)
	}

	//密码保存到内存
	key.WalletPasswd = m5passwd
	key.PasswdPath = path

	return nil
}

func ResetPasswd(passwd []byte) error {
	err := os.Remove(key.PasswdPath)
	if err != nil {
		return err
	}

	err = SetupPasswd(passwd, key.PasswdPath)
	if err != nil {
		return err
	}

	return nil
}

func ClearPasswd() error {
	err := os.Remove(key.PasswdPath)
	if err != nil {
		return err
	}
	key.WalletPasswd = ""
	key.PasswdPath = ""
	return nil
}

func CheckPasswd(passwd []byte) error {
	fstat, err := os.Stat(key.PasswdPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("opening file '%s': file info not found", key.PasswdPath)
	} else if err != nil {
		return fmt.Errorf("opening file '%s': %w", key.PasswdPath, err)
	}

	if fstat.Mode()&0077 != 0 {
		return fmt.Errorf("permissions of key: '%s' are too relaxed, required: 0600, got: %#o", key.PasswdPath, fstat.Mode())
	}

	if fstat.Mode()&0077 != 0 {
		return xerrors.Errorf("permissions of key: '%s' are too relaxed, required: 0600, got: %#o", key.PasswdPath, fstat.Mode())
	}

	file, err := os.Open(key.PasswdPath)
	if err != nil {
		return xerrors.Errorf("opening file '%s': %w", key.PasswdPath, err)
	}
	defer file.Close() //nolint:errcheck

	data, err := ioutil.ReadAll(file)
	if err != nil {
		return xerrors.Errorf("reading file '%s': %w", key.PasswdPath, err)
	}

	//读取加密check消息
	passwd = completionPwd(passwd)
	/* #nosec G401 */
	m5 := md5.Sum(passwd)
	text, err := key.AESDecrypt(m5[:], data[64:])
	if err != nil {
		return err
	}

	//验证check消息
	str := string(text)
	if walletCheckMsg != str {
		return xerrors.Errorf("check passwd is failed")
	}
	//密码保存到内存
	if key.IsLock() {
		key.WalletPasswd = hex.EncodeToString(m5[:])
	}
	return nil
}

func GetSetupState(path string) bool {
	fstat, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	} else if err != nil {
		return false
	}

	if fstat.Mode()&0077 != 0 {
		return false
	}

	file, err := os.Open(path)
	if err != nil {
		log.Infof("opening file '%s': %w", path, err)
		return false
	}
	defer file.Close() //nolint:errcheck

	data, err := ioutil.ReadAll(file)
	if err != nil {
		log.Infof("reading file '%s': %w", path, err)
		return false
	}

	//读取密码
	saltkey := completionPwd([]byte(walletSaltPwd))
	/* #nosec G401 */
	saltm5 := md5.Sum(saltkey)
	m5passwd, err := key.AESDecrypt(saltm5[:], data[:64])
	if err != nil {
		log.Infof("err: %v", err)
		return false
	}

	//用读取密码去验证加密check消息
	m5pwdstr := string(m5passwd[:32])
	m5pwd, _ := hex.DecodeString(m5pwdstr)
	text, err := key.AESDecrypt(m5pwd[:16], data[64:])
	if err != nil {
		log.Infof("err: %v", err)
		return false
	}
	str := string(text)
	if walletCheckMsg != str {
		log.Infof("check passwd is failed")
		return false
	}

	key.PasswdPath = path
	key.WalletPasswd = m5pwdstr
	return true
}

// GetSetupStateForLocal only lotus-wallet use
//
// check encryption status
func GetSetupStateForLocal(path string) bool {
	fstat, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	} else if err != nil {
		return false
	}

	if fstat.Mode()&0077 != 0 {
		return false
	}

	return true
}

func RegexpPasswd(passwd string) error {
	// if ok, _ := regexp.MatchString(`^[a-zA-Z].{5,15}`, passwd); len(passwd) > 16 || !ok {
	if ok, _ := regexp.MatchString(`^[a-zA-Z].[0-9A-Za-z!@#$%^&*]{4,15}`, passwd); len(passwd) > 16 || !ok {
		// if ok, _ := regexp.MatchString(`^(?![0-9]+$)(?![a-zA-Z]+$)[a-zA-Z][0-9A-Za-z!@#$%^&*]{5,15}$`, passwd); len(passwd) > 18 || !ok {
		return fmt.Errorf("invalid password format. (The beginning of the letter, 6 to 16 characters.)")
	}
	return nil
}
