package lp2p

import (
	"fmt"
	"strings"

	"github.com/libp2p/go-libp2p"
	pnet "github.com/libp2p/go-libp2p-pnet"
)

var LotusKey = "/key/swarm/psk/1.0.0/\n/base16/\n20c72388e6299c7bbc1b501fdcc8abe4f89f798e9b93b2d2bc02e3c29b6a088e"

type PNetFingerprint []byte

func PNet() (opts Libp2pOpts, fp PNetFingerprint, err error) {
	protec, err := pnet.NewProtector(strings.NewReader(LotusKey))
	if err != nil {
		return opts, nil, fmt.Errorf("failed to configure private network: %s", err)
	}
	fp = protec.Fingerprint()

	opts.Opts = append(opts.Opts, libp2p.PrivateNetwork(protec))
	return opts, fp, nil
}

/*
func PNetChecker(repo repo.Repo, ph host.Host, lc fx.Lifecycle) error {
	// TODO: better check?
	swarmkey, err := repo.SwarmKey()
	if err != nil || swarmkey == nil {
		return err
	}

	done := make(chan struct{})
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			go func() {
				t := time.NewTicker(30 * time.Second)
				defer t.Stop()

				<-t.C // swallow one tick
				for {
					select {
					case <-t.C:
						if len(ph.Network().Peers()) == 0 {
							log.Warn("We are in private network and have no peers.")
							log.Warn("This might be configuration mistake.")
						}
					case <-done:
						return
					}
				}
			}()
			return nil
		},
		OnStop: func(_ context.Context) error {
			close(done)
			return nil
		},
	})
	return nil
}
*/
