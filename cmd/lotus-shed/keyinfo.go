package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"text/template"

	"github.com/urfave/cli/v2"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/node/modules/lp2p"
	"github.com/filecoin-project/lotus/node/repo"

	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
)

var validTypes = []string{wallet.KTBLS, wallet.KTSecp256k1, lp2p.KTLibp2pHost}

type keyInfoOutput struct {
	Type      string
	Address   string
	PublicKey string
}

var keyinfoCmd = &cli.Command{
	Name:  "keyinfo",
	Usage: "work with lotus keyinfo files (wallets and libp2p host keys)",
	Description: `The subcommands of keyinfo provide helpful tools for working with keyinfo files without
   having to run the lotus daemon.`,
	Subcommands: []*cli.Command{
		keyinfoNewCmd,
		keyinfoInfoCmd,
		keyinfoImportCmd,
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
		if cctx.Args().Len() == 0 {
			input = os.Stdin
		} else {
			var err error
			input, err = os.Open(cctx.Args().First())
			if err != nil {
				return err
			}
		}

		encoded, err := ioutil.ReadAll(input)
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

		defer lkrepo.Close()

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
		case wallet.KTSecp256k1:
		case wallet.KTBLS:
			w, err := wallet.NewWallet(keystore)
			if err != nil {
				return err
			}

			addr, err := w.Import(&keyInfo)
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

   The following fields can be retrived through this command
     Type
     Address
     PublicKey

   The PublicKey value will be printed base64 encoded using golangs StdEncoding

   Examples

   Retreive the address of a lotus wallet
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
		if cctx.Args().Len() == 0 {
			input = os.Stdin
		} else {
			var err error
			input, err = os.Open(cctx.Args().First())
			if err != nil {
				return err
			}
		}

		encoded, err := ioutil.ReadAll(input)
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
		case wallet.KTSecp256k1:
		case wallet.KTBLS:
			kio.Type = keyInfo.Type

			key, err := wallet.NewKey(keyInfo)
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
   the 'lotus wallet import' command. Or imported to a non-running / unitialized repo using
   the 'lotus-shed keyinfo import' command. Libp2p host keys can only be imported using lotus-shed
   as lotus itself does not provide this functionality at the moment.`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "output",
			Value: "<type>-<addr>.keyinfo",
			Usage: "output file formt",
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

		keyType := cctx.Args().First()
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
		case wallet.KTSecp256k1:
		case wallet.KTBLS:
			key, err := wallet.GenerateKey(wallet.ActSigType(keyType))
			if err != nil {
				return err
			}

			keyAddr = key.Address.String()
			keyInfo = key.KeyInfo

			break
		}

		filename := flagOutput
		filename = strings.ReplaceAll(filename, "<addr>", keyAddr)
		filename = strings.ReplaceAll(filename, "<type>", keyType)

		file, err := os.Create(filename)
		if err != nil {
			return err
		}

		defer func() {
			if err := file.Close(); err != nil {
				log.Warnf("failed to close output file: %w", err)
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
