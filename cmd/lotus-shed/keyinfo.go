package main

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"text/template"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-base32"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/delegated"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/lp2p"
	"github.com/filecoin-project/lotus/node/repo"
)

var validTypes = []types.KeyType{types.KTBLS, types.KTSecp256k1, lp2p.KTLibp2pHost}

type keyInfoOutput struct {
	Type      types.KeyType
	Address   string
	PublicKey string
}

var keyinfoCmd = &cli.Command{
	Name:    "key-info",
	Aliases: []string{"keyinfo"},
	Usage:   "work with lotus keyinfo files (wallets and libp2p host keys)",
	Description: `The subcommands of keyinfo provide helpful tools for working with keyinfo files without
   having to run the lotus daemon.`,
	Subcommands: []*cli.Command{
		keyinfoNewCmd,
		keyinfoInfoCmd,
		keyinfoImportCmd,
		keyinfoVerifyCmd,
	},
}

var keyinfoVerifyCmd = &cli.Command{
	Name:  "verify",
	Usage: "verify the filename of a keystore object on disk with it's contents",
	Description: `Keystore objects are base32 encoded strings, with wallets being dynamically named via
   the wallet address. This command can ensure that the naming of these keystore objects are correct`,
	Action: func(cctx *cli.Context) error {
		filePath := cctx.Args().First()
		fileName := path.Base(filePath)

		inputFile, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer inputFile.Close() //nolint:errcheck
		input := bufio.NewReader(inputFile)

		keyContent, err := io.ReadAll(input)
		if err != nil {
			return err
		}

		var keyInfo types.KeyInfo
		if err := json.Unmarshal(keyContent, &keyInfo); err != nil {
			return err
		}

		switch keyInfo.Type {
		case lp2p.KTLibp2pHost:
			name, err := base32.RawStdEncoding.DecodeString(fileName)
			if err != nil {
				return xerrors.Errorf("decoding key: '%s': %w", fileName, err)
			}

			if types.KeyType(name) != keyInfo.Type {
				return fmt.Errorf("%s of type %s is incorrect", fileName, keyInfo.Type)
			}
		case modules.KTJwtHmacSecret:
			name, err := base32.RawStdEncoding.DecodeString(fileName)
			if err != nil {
				return xerrors.Errorf("decoding key: '%s': %w", fileName, err)
			}

			if string(name) != modules.JWTSecretName {
				return fmt.Errorf("%s of type %s is incorrect", fileName, keyInfo.Type)
			}
		case types.KTSecp256k1, types.KTBLS:
			keystore := wallet.NewMemKeyStore()
			w, err := wallet.NewWallet(keystore)
			if err != nil {
				return err
			}

			if _, err := w.WalletImport(cctx.Context, &keyInfo); err != nil {
				return err
			}

			list, err := keystore.List()
			if err != nil {
				return err
			}

			if len(list) != 1 {
				return fmt.Errorf("unexpected number of keys, expected 1, found %d", len(list))
			}

			name, err := base32.RawStdEncoding.DecodeString(fileName)
			if err != nil {
				return xerrors.Errorf("decoding key: '%s': %w", fileName, err)
			}

			if string(name) != list[0] {
				return fmt.Errorf("%s of type %s; file is named for %s, but key is actually %s", fileName, keyInfo.Type, string(name), list[0])
			}

			break
		default:
			return fmt.Errorf("unknown keytype %s", keyInfo.Type)
		}

		return nil
	},
}

var keyinfoImportCmd = &cli.Command{
	Name:  "import",
	Usage: "import a keyinfo file into a lotus repository",
	Description: `The import command provides a way to import keyfiles into a lotus repository
   without running the daemon.

   Note: The LOTUS_PATH directory must be created. This command will not create this directory for you.

   Examples

   env LOTUS_PATH=/var/lib/lotus lotus-shed keyinfo import libp2p-host.keyinfo`,
	Action: func(cctx *cli.Context) error {
		flagRepo := cctx.String("repo")

		var input io.Reader
		if cctx.NArg() == 0 {
			input = os.Stdin
		} else {
			var err error
			inputFile, err := os.Open(cctx.Args().First())
			if err != nil {
				return err
			}
			defer inputFile.Close() //nolint:errcheck
			input = bufio.NewReader(inputFile)
		}

		encoded, err := io.ReadAll(input)
		if err != nil {
			return err
		}

		decoded, err := hex.DecodeString(strings.TrimSpace(string(encoded)))
		if err != nil {
			return err
		}

		var keyInfo types.KeyInfo
		if err := json.Unmarshal(decoded, &keyInfo); err != nil {
			return err
		}

		fsrepo, err := repo.NewFS(flagRepo)
		if err != nil {
			return err
		}

		lkrepo, err := fsrepo.Lock(repo.FullNode)
		if err != nil {
			return err
		}

		defer lkrepo.Close() //nolint:errcheck

		keystore, err := lkrepo.KeyStore()
		if err != nil {
			return err
		}

		switch keyInfo.Type {
		case lp2p.KTLibp2pHost:
			if err := keystore.Put(lp2p.KLibp2pHost, keyInfo); err != nil {
				return err
			}

			sk, err := crypto.UnmarshalPrivateKey(keyInfo.PrivateKey)
			if err != nil {
				return err
			}

			peerid, err := peer.IDFromPrivateKey(sk)
			if err != nil {
				return err
			}

			fmt.Printf("%s\n", peerid.String())

			break
		case types.KTSecp256k1, types.KTBLS:
			w, err := wallet.NewWallet(keystore)
			if err != nil {
				return err
			}

			addr, err := w.WalletImport(cctx.Context, &keyInfo)
			if err != nil {
				return err
			}

			fmt.Printf("%s\n", addr.String())
		}

		return nil
	},
}

var keyinfoInfoCmd = &cli.Command{
	Name:  "info",
	Usage: "print information about a keyinfo file",
	Description: `The info command prints additional information about a key which can't easily
   be retrieved by inspecting the file itself.

   The 'format' flag takes a golang text/template template as its value.

   The following fields can be retrieved through this command
     Type
     Address
     PublicKey

   The PublicKey value will be printed base64 encoded using golangs StdEncoding

   Examples

   Retrieve the address of a lotus wallet
   lotus-shed keyinfo info --format '{{ .Address }}' wallet.keyinfo
   `,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "format",
			Value: "{{ .Type }} {{ .Address }}",
			Usage: "specify which output columns to print",
		},
	},
	Action: func(cctx *cli.Context) error {
		format := cctx.String("format")

		var input io.Reader
		if cctx.NArg() == 0 {
			input = os.Stdin
		} else {
			var err error
			inputFile, err := os.Open(cctx.Args().First())
			if err != nil {
				return err
			}
			defer inputFile.Close() //nolint:errcheck
			input = bufio.NewReader(inputFile)
		}

		encoded, err := io.ReadAll(input)
		if err != nil {
			return err
		}

		decoded, err := hex.DecodeString(strings.TrimSpace(string(encoded)))
		if err != nil {
			return err
		}

		var keyInfo types.KeyInfo
		if err := json.Unmarshal(decoded, &keyInfo); err != nil {
			return err
		}

		var kio keyInfoOutput

		switch keyInfo.Type {
		case lp2p.KTLibp2pHost:
			kio.Type = keyInfo.Type

			sk, err := crypto.UnmarshalPrivateKey(keyInfo.PrivateKey)
			if err != nil {
				return err
			}

			pk := sk.GetPublic()

			peerid, err := peer.IDFromPrivateKey(sk)
			if err != nil {
				return err
			}

			pkBytes, err := pk.Raw()
			if err != nil {
				return err
			}

			kio.Address = peerid.String()
			kio.PublicKey = base64.StdEncoding.EncodeToString(pkBytes)

			break
		case types.KTSecp256k1, types.KTBLS:
			kio.Type = keyInfo.Type

			key, err := key.NewKey(keyInfo)
			if err != nil {
				return err
			}

			kio.Address = key.Address.String()
			kio.PublicKey = base64.StdEncoding.EncodeToString(key.PublicKey)
		}

		tmpl, err := template.New("output").Parse(format)
		if err != nil {
			return err
		}

		return tmpl.Execute(os.Stdout, kio)
	},
}

var keyinfoNewCmd = &cli.Command{
	Name:      "new",
	Usage:     "create a new keyinfo file of the provided type",
	ArgsUsage: "[bls|secp256k1|libp2p-host]",
	Description: `Keyinfo files are base16 encoded json structures containing a type
   string value, and a base64 encoded private key.

   Both the bls and secp256k1 keyfiles can be imported into a running lotus daemon using
   the 'lotus wallet import' command. Or imported to a non-running / uninitialized repo using
   the 'lotus-shed keyinfo import' command. Libp2p host keys can only be imported using lotus-shed
   as lotus itself does not provide this functionality at the moment.`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "output",
			Value: "<type>-<addr>.keyinfo",
			Usage: "output file format",
		},
		&cli.BoolFlag{
			Name:  "silent",
			Value: false,
			Usage: "do not print the address to stdout",
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("please specify a type to generate")
		}

		keyType := types.KeyType(cctx.Args().First())
		flagOutput := cctx.String("output")

		if i := SliceIndex(len(validTypes), func(i int) bool {
			if keyType == validTypes[i] {
				return true
			}
			return false
		}); i == -1 {
			return fmt.Errorf("invalid key type argument provided '%s'", keyType)
		}

		keystore := wallet.NewMemKeyStore()

		var keyAddr string
		var keyInfo types.KeyInfo

		switch keyType {
		case lp2p.KTLibp2pHost:
			sk, err := lp2p.PrivKey(keystore)
			if err != nil {
				return err
			}

			ki, err := keystore.Get(lp2p.KLibp2pHost)
			if err != nil {
				return err
			}

			peerid, err := peer.IDFromPrivateKey(sk)
			if err != nil {
				return err
			}

			keyAddr = peerid.String()
			keyInfo = ki

			break
		case types.KTSecp256k1, types.KTBLS:
			key, err := key.GenerateKey(keyType)
			if err != nil {
				return err
			}

			keyAddr = key.Address.String()
			keyInfo = key.KeyInfo

			break
		}

		filename := flagOutput
		filename = strings.ReplaceAll(filename, "<addr>", keyAddr)
		filename = strings.ReplaceAll(filename, "<type>", string(keyType))

		file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}

		defer func() {
			if err := file.Close(); err != nil {
				log.Warnf("failed to close output file: %v", err)
			}
		}()

		bytes, err := json.Marshal(keyInfo)
		if err != nil {
			return err
		}

		encoded := hex.EncodeToString(bytes)
		if _, err := file.Write([]byte(encoded)); err != nil {
			return err
		}

		if !cctx.Bool("silent") {
			fmt.Println(keyAddr)
		}

		return nil
	},
}

func SliceIndex(length int, fn func(i int) bool) int {
	for i := 0; i < length; i++ {
		if fn(i) {
			return i
		}
	}

	return -1
}
