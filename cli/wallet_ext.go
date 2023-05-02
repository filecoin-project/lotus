package cli

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/chain/wallet"

	// wallet-security api
	lapi "github.com/filecoin-project/lotus/api"
)

// wallet-security func cmd
var walletPasswdCmd = &cli.Command{
	Name:  "passwd",
	Usage: "Manage wallet passwd info",
	Subcommands: []*cli.Command{
		walletAddPasswd,
		walletResetPasswd,
		walletClearPasswd,
	},
}
var walletAddPasswd = &cli.Command{
	Name:  "add",
	Usage: "Add wallet password",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if wallet.GetSetupStateForLocal(getWalletPwdRepo(cctx)) {
			fmt.Println("passwd is setup,no need to setup again")
			return nil
		}

		// passwd := cctx.String("passwd")
		passwd := wallet.Prompt("Enter your password:\n")
		if passwd == "" {
			return xerrors.Errorf("must enter your passwd")
		}
		if err := wallet.RegexpPasswd(passwd); err != nil {
			return err
		}

		passwd_ag := wallet.Prompt("Enter your password again:\n")
		if passwd_ag == "" {
			return fmt.Errorf("must enter your passwd")
		}
		if err := wallet.RegexpPasswd(passwd_ag); err != nil {
			return err
		}
		if passwd != passwd_ag {
			return fmt.Errorf("the two passwords do not match")
		}

		_, err = api.WalletCustomMethod(ctx, lapi.WalletAddPasswd, []interface{}{passwd, getWalletPwdRepo(cctx)})
		if err != nil {
			fmt.Println(err.Error())
			return err
		}
		fmt.Printf("add wallet password success. \n")
		return nil
	},
}
var walletResetPasswd = &cli.Command{
	Name:  "reset",
	Usage: "Reset wallet password",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)
		if !wallet.GetSetupStateForLocal(getWalletPwdRepo(cctx)) {
			fmt.Println("passwd is not setup")
			return nil
		}

		// passwd := cctx.String("passwd")
		passwd := wallet.Prompt("Enter your old Password:\n")
		if passwd == "" {
			return xerrors.Errorf("Must enter your old passwd")
		}
		if err := wallet.RegexpPasswd(passwd); err != nil {
			return err
		}

		rest, _ := api.WalletCustomMethod(ctx, lapi.WalletCheckPasswd, []interface{}{passwd})
		if !rest.(bool) {
			return fmt.Errorf("check passwd is failed")
		}

		newPasswd := wallet.Prompt("Enter your new Password:\n")
		if passwd == "" {
			return xerrors.Errorf("Must enter your new passwd")
		}
		if err := wallet.RegexpPasswd(newPasswd); err != nil {
			return fmt.Errorf("new passwd : %w", err)
		}

		newPasswd_ag := wallet.Prompt("Enter your password again:\n")
		if newPasswd_ag == "" {
			return fmt.Errorf("must enter your passwd")
		}
		if err := wallet.RegexpPasswd(newPasswd_ag); err != nil {
			return err
		}
		if newPasswd != newPasswd_ag {
			return fmt.Errorf("the two passwords do not match")
		}

		_, err = api.WalletCustomMethod(ctx, lapi.WalletResetPasswd, []interface{}{passwd, newPasswd})
		if err != nil {
			return err
		}

		fmt.Printf("wallet passwd change success. \n")
		return nil
	},
}
var walletClearPasswd = &cli.Command{
	Name:  "clear",
	Usage: "Clear wallet password",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)
		if !wallet.GetSetupStateForLocal(getWalletPwdRepo(cctx)) {
			fmt.Println("passwd is not setup")
			return nil
		}

		// passwd := cctx.String("passwd")
		passwd := wallet.Prompt("Enter your Password:\n")
		if passwd == "" {
			return xerrors.Errorf("Must enter your passwd")
		}
		if err := wallet.RegexpPasswd(passwd); err != nil {
			return err
		}

		_, err = api.WalletCustomMethod(ctx, lapi.WalletClearPasswd, []interface{}{passwd})
		if err != nil {
			return err
		}

		fmt.Printf("wallet passwd clear success. \n")
		return nil
	},
}

var walletEncrypt = &cli.Command{
	Name:      "encrypt",
	Usage:     "encrypt wallet account",
	ArgsUsage: "[address]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)
		if !wallet.GetSetupStateForLocal(getWalletPwdRepo(cctx)) {
			fmt.Println("passwd is not setup")
			return nil
		}

		if cctx.Args().First() == "all" {
			// wallet-security encrypt all
			encryptAddrs := make([]lapi.AddrListEncrypt, 0)
			rest, err := api.WalletCustomMethod(ctx, lapi.WalletListForEnc, []interface{}{})
			if rest != nil {
				if err != nil {
					return err
				}
				addrs, _ := json.Marshal(rest)
				if err := json.Unmarshal(addrs, &encryptAddrs); err != nil {
					return err
				}
			} else {
				l, err := api.WalletList(ctx)
				if err != nil {
					return err
				}
				for _, v := range l {
					encryptAddrs = append(encryptAddrs, lapi.AddrListEncrypt{
						Addr:    v,
						Encrypt: false,
					})
				}
			}

			// passwd := cctx.String("passwd")
			passwd := wallet.Prompt("Enter your Password:\n")
			if passwd == "" {
				return xerrors.Errorf("Must enter your passwd")
			}
			if err := wallet.RegexpPasswd(passwd); err != nil {
				return err
			}

			for _, encryptAddr := range encryptAddrs {
				if encryptAddr.Encrypt {
					continue
				}

				rest, _ := api.WalletCustomMethod(ctx, lapi.WalletIsEncrypt, []interface{}{encryptAddr.Addr})
				if rest != nil && !rest.(bool) {
					_, err = api.WalletCustomMethod(ctx, lapi.WalletEncrypt, []interface{}{encryptAddr.Addr})
					if err != nil {
						return err
					}

					fmt.Printf("Wallet %v encrypt success \n", encryptAddr.Addr)
				} else {
					fmt.Printf("Wallet %v is encrypted \n", encryptAddr.Addr)
				}
			}
		} else {
			addr, err := address.NewFromString(cctx.Args().First())
			if err != nil {
				return err
			}

			rest, _ := api.WalletCustomMethod(ctx, lapi.WalletIsEncrypt, []interface{}{addr})
			if rest != nil && !rest.(bool) {

				_, err = api.WalletCustomMethod(ctx, lapi.WalletEncrypt, []interface{}{addr})
				if err != nil {
					return err
				}

				fmt.Printf("Wallet encrypt success \n")
			} else {
				fmt.Printf("Wallet is encrypted \n")
			}
		}
		return nil
	},
}
var walletDecrypt = &cli.Command{
	Name:      "decrypt",
	Usage:     "decrypt wallet account",
	ArgsUsage: "[address]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)
		if !wallet.GetSetupStateForLocal(getWalletPwdRepo(cctx)) {
			fmt.Println("passwd is not setup")
			return nil
		}

		if cctx.Args().First() == "all" {
			// wallet-security encrypt all
			encryptAddrs := make([]lapi.AddrListEncrypt, 0)
			rest, err := api.WalletCustomMethod(ctx, lapi.WalletListForEnc, []interface{}{})
			if rest != nil {
				if err != nil {
					return err
				}
				addrs, _ := json.Marshal(rest)
				json.Unmarshal(addrs, &encryptAddrs)
			} else {
				l, err := api.WalletList(ctx)
				if err != nil {
					return err
				}
				for _, v := range l {
					encryptAddrs = append(encryptAddrs, lapi.AddrListEncrypt{
						Addr:    v,
						Encrypt: false,
					})
				}
			}

			// passwd := cctx.String("passwd")
			passwd := wallet.Prompt("Enter your Password:\n")
			if passwd == "" {
				return xerrors.Errorf("Must enter your passwd")
			}
			if err := wallet.RegexpPasswd(passwd); err != nil {
				return err
			}

			for _, encryptAddr := range encryptAddrs {
				if !encryptAddr.Encrypt {
					continue
				}

				rest, _ := api.WalletCustomMethod(ctx, lapi.WalletIsEncrypt, []interface{}{encryptAddr.Addr})
				if rest != nil && rest.(bool) {
					_, err = api.WalletCustomMethod(ctx, lapi.WalletDecrypt, []interface{}{encryptAddr.Addr, passwd})
					if err != nil {
						return err
					}

					fmt.Printf("Wallet %v decrypt success \n", encryptAddr.Addr)
				} else {
					fmt.Printf("Wallet %v is not encrypted \n", encryptAddr.Addr)
				}
			}
		} else {
			addr, err := address.NewFromString(cctx.Args().First())
			if err != nil {
				return err
			}

			rest, _ := api.WalletCustomMethod(ctx, lapi.WalletIsEncrypt, []interface{}{addr})
			if rest != nil && rest.(bool) {
				// passwd := cctx.String("passwd")
				passwd := wallet.Prompt("Enter your Password:\n")
				if passwd == "" {
					return xerrors.Errorf("Must enter your passwd")
				}
				if err := wallet.RegexpPasswd(passwd); err != nil {
					return err
				}

				_, err = api.WalletCustomMethod(ctx, lapi.WalletDecrypt, []interface{}{addr, passwd})
				if err != nil {
					return err
				}

				fmt.Printf("Wallet decrypt success \n")
			} else {
				fmt.Printf("Wallet is not encrypted \n")
			}
		}
		return nil
	},
}

// wallet-security pwd func
func getWalletPwdRepo(cctx *cli.Context) string {
	passwdPath, err := homedir.Expand(cctx.String("repo"))
	if err != nil {
		return ""
	}
	return passwdPath + "/keystore/passwd"
}

// wallet-extand mark func cmd
var walletMarkCmd = &cli.Command{
	Name:  "mark",
	Usage: "Manage wallet mark info",
	Subcommands: []*cli.Command{
		walletAddMarkCmd,
		walletDelMarkCmd,
		walletClearMarkCmd,
	},
}
var walletAddMarkCmd = &cli.Command{
	Name:      "add",
	Usage:     "Add/Update wallet mark",
	ArgsUsage: "[address]",
	Action: func(cctx *cli.Context) error {
		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		mark := wallet.PromptSimple("Enter mark:\n")
		if mark == "" {
			return xerrors.Errorf("Enter mark is empty.")
		}

		var keyMark map[string]string
		var keymarkPath string = getWalletMarkRepo(cctx)
		_, err = os.Stat(keymarkPath)
		if err == nil {
			sector, err := ioutil.ReadFile(keymarkPath)
			if err == nil {
				err = json.Unmarshal(sector, &keyMark)
				if err != nil {
					keyMark = map[string]string{}
				}
			} else {
				keyMark = map[string]string{}
			}
		} else {
			keyMark = map[string]string{}
		}

		keyMark[addr.String()] = mark

		err = saveWalletMarkFile(cctx, keyMark)
		if err != nil {
			return err
		}

		fmt.Printf("wallet add mark success. %v:%v \n", addr.String(), mark)
		return nil
	},
}
var walletDelMarkCmd = &cli.Command{
	Name:      "del",
	Usage:     "Delete wallet mark",
	ArgsUsage: "[address]",
	Action: func(cctx *cli.Context) error {
		if !wallet.GetSetupStateForLocal(getWalletMarkRepo(cctx)) {
			fmt.Println("mark is not setup")
			return nil
		}

		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		var keyMark map[string]string
		var keymarkPath string = getWalletMarkRepo(cctx)
		_, err = os.Stat(keymarkPath)
		if err == nil {
			sector, err := ioutil.ReadFile(keymarkPath)
			if err == nil {
				err = json.Unmarshal(sector, &keyMark)
				if err != nil {
					keyMark = map[string]string{}
				}
			} else {
				keyMark = map[string]string{}
			}
		} else {
			keyMark = map[string]string{}
		}

		for k := range keyMark {
			if k == addr.String() {
				delete(keyMark, addr.String())
			}
		}

		err = saveWalletMarkFile(cctx, keyMark)
		if err != nil {
			return err
		}

		fmt.Printf("wallet delete mark success. %v \n", addr.String())
		return nil
	},
}
var walletClearMarkCmd = &cli.Command{
	Name:  "clear",
	Usage: "Clear wallet mark",
	Action: func(cctx *cli.Context) error {
		if !wallet.GetSetupStateForLocal(getWalletMarkRepo(cctx)) {
			fmt.Println("mark is not setup")
			return nil
		}

		var keyMark map[string]string
		var keymarkPath string = getWalletMarkRepo(cctx)
		_, err := os.Stat(keymarkPath)
		if err == nil {
			sector, err := ioutil.ReadFile(keymarkPath)
			if err == nil {
				err = json.Unmarshal(sector, &keyMark)
				if err != nil {
					keyMark = map[string]string{}
				}
			} else {
				keyMark = map[string]string{}
			}
		} else {
			keyMark = map[string]string{}
		}

		for k := range keyMark {
			delete(keyMark, k)
		}

		err = saveWalletMarkFile(cctx, keyMark)
		if err != nil {
			return err
		}

		fmt.Printf("wallet clear mark success. \n")
		return nil
	},
}

// wallet-extand mark func
func getWalletMarkRepo(cctx *cli.Context) string {
	keymarkPath, err := homedir.Expand(cctx.String("repo"))
	if err != nil {
		return ""
	}
	return keymarkPath + "/keystore/keymark"
}

func saveWalletMarkFile(cctx *cli.Context, keyMark map[string]string) error {
	var keymarkPath string = getWalletMarkRepo(cctx)
	f, err := os.OpenFile(keymarkPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY|os.O_SYNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	sector, err := json.MarshalIndent(keyMark, "", "\t")
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(keymarkPath, sector, 0600)
	if err != nil {
		return xerrors.Errorf("writing file '%s': %w", keymarkPath, err)
	}
	return nil
}
