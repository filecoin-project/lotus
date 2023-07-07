package modules_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/go-datastore"
	provider "github.com/ipni/index-provider"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

func Test_IndexProviderTopic(t *testing.T) {
	tests := []struct {
		name                 string
		givenAllowedTopics   []string
		givenConfiguredTopic string
		givenNetworkName     dtypes.NetworkName
		wantErr              string
	}{
		{
			name:                 "Joins configured topic when allowed",
			givenAllowedTopics:   []string{"fish"},
			givenConfiguredTopic: "fish",
		},
		{
			name:               "Joins topic inferred from network name when allowed",
			givenAllowedTopics: []string{"/indexer/ingest/fish"},
			givenNetworkName:   "fish",
		},
		{
			name:                 "Fails to join configured topic when disallowed",
			givenAllowedTopics:   []string{"/indexer/ingest/fish"},
			givenConfiguredTopic: "lobster",
			wantErr:              "joining indexer topic lobster: topic is not allowed by the subscription filter",
		},
		{
			name:               "Fails to join topic inferred from network name when disallowed",
			givenAllowedTopics: []string{"/indexer/ingest/fish"},
			givenNetworkName:   "lobster",
			wantErr:            "joining indexer topic /indexer/ingest/lobster: topic is not allowed by the subscription filter",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			h, err := libp2p.New()
			require.NoError(t, err)
			defer func() {
				require.NoError(t, h.Close())
			}()

			filter := pubsub.WithSubscriptionFilter(pubsub.NewAllowlistSubscriptionFilter(test.givenAllowedTopics...))
			ps, err := pubsub.NewGossipSub(ctx, h, filter)
			require.NoError(t, err)

			app := fx.New(
				fx.Provide(
					func() host.Host { return h },
					func() dtypes.NetworkName { return test.givenNetworkName },
					func() dtypes.MinerAddress { return dtypes.MinerAddress(address.TestAddress) },
					func() dtypes.ProviderDataTransfer { return nil },
					func() *pubsub.PubSub { return ps },
					func() dtypes.MetadataDS { return datastore.NewMapDatastore() },
					modules.IndexProvider(config.IndexProviderConfig{
						Enable:           true,
						TopicName:        test.givenConfiguredTopic,
						EntriesChunkSize: 16384,
					}),
				),
				fx.Invoke(func(p provider.Interface) {}),
			)
			err = app.Start(ctx)

			if test.wantErr == "" {
				require.NoError(t, err)
				err = app.Stop(ctx)
				require.NoError(t, err)
			} else {
				require.True(t, strings.HasSuffix(err.Error(), test.wantErr))
			}
		})
	}
}
