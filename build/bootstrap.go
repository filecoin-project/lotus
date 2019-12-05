package build

import (
	"context"
	"os"
	"strings"

	"github.com/filecoin-project/lotus/lib/addrutil"
	"golang.org/x/xerrors"

	rice "github.com/GeertJohan/go.rice"
	"github.com/libp2p/go-libp2p-core/peer"
)

func BuiltinBootstrap() ([]peer.AddrInfo, error) {
	var out []peer.AddrInfo

	b := rice.MustFindBox("bootstrap")
	err := b.Walk("", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return xerrors.Errorf("failed to walk box: %w", err)
		}

		if !strings.HasSuffix(path, ".pi") {
			return nil
		}
		spi := b.MustString(path)
		if spi == "" {
			return nil
		}
		pi, err := addrutil.ParseAddresses(context.TODO(), strings.Split(strings.TrimSpace(spi), "\n"))
		out = append(out, pi...)
		return err
	})
	return out, err
}
