package lf3

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-f3/ec"
	"github.com/filecoin-project/go-f3/manifest"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/lib/must"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
)

type headGetter store.ChainStore

func (hg *headGetter) GetHead(context.Context) (ec.TipSet, error) {
	head := (*store.ChainStore)(hg).GetHeaviestTipSet()
	if head == nil {
		return nil, xerrors.New("no heaviest tipset")
	}
	return &f3TipSet{TipSet: head}, nil
}

// Determines the max. number of configuration changes
// that are allowed for the dynamic manifest.
// If the manifest changes more than this number, the F3
// message topic will be filtered
var MaxDynamicManifestChangesAllowed = 1000

func NewManifestProvider(mctx helpers.MetricsCtx, config *Config, cs *store.ChainStore, ps *pubsub.PubSub, mds dtypes.MetadataDS) (prov manifest.ManifestProvider, err error) {
	if config.DynamicManifestProvider == "" || !build.IsF3PassiveTestingEnabled() {
		if config.StaticManifest == nil {
			return manifest.NoopManifestProvider{}, nil
		}
		return manifest.NewStaticManifestProvider(config.StaticManifest)
	}

	opts := []manifest.DynamicManifestProviderOption{
		manifest.DynamicManifestProviderWithDatastore(
			namespace.Wrap(mds, datastore.NewKey("/f3-dynamic-manifest")),
		),
	}

	if config.StaticManifest != nil {
		opts = append(opts,
			manifest.DynamicManifestProviderWithInitialManifest(config.StaticManifest),
		)
	}

	if config.AllowDynamicFinalize {
		log.Error("dynamic F3 manifests are allowed to finalize tipsets, do not enable this in production!")
	}

	networkNameBase := config.BaseNetworkName + "/"
	filter := func(m *manifest.Manifest) error {
		if m.EC.Finalize {
			if !config.AllowDynamicFinalize {
				return fmt.Errorf("refusing dynamic manifest that finalizes tipsets")
			}
			log.Error("WARNING: loading a dynamic F3 manifest that will finalize new tipsets")
		}
		if !strings.HasPrefix(string(m.NetworkName), string(networkNameBase)) {
			return fmt.Errorf(
				"refusing dynamic manifest with network name %q, must start with %q",
				m.NetworkName,
				networkNameBase,
			)
		}
		return nil
	}
	opts = append(opts,
		manifest.DynamicManifestProviderWithFilter(filter),
	)

	prov, err = manifest.NewDynamicManifestProvider(ps, config.DynamicManifestProvider, opts...)
	if err != nil {
		return nil, err
	}
	if config.PrioritizeStaticManifest && config.StaticManifest != nil {
		prov, err = manifest.NewFusingManifestProvider(mctx,
			(*headGetter)(cs), prov, config.StaticManifest)
	}
	return prov, err
}

type StateCaller interface {
	StateCall(ctx context.Context, msg *types.Message, tsk types.TipSetKey) (res *api.InvocResult, err error)
}

type ContractManifestProvider struct {
	address     string
	stateCaller StateCaller
	CheckPeriod time.Duration

	manifestChanges chan *manifest.Manifest

	errgrp     *errgroup.Group
	runningCtx context.Context
	cancel     context.CancelFunc
}

func NewContractManifestProvider(mctx helpers.MetricsCtx, config *Config, stateCaller StateCaller) (*ContractManifestProvider, error) {
	ctx, cancel := context.WithCancel(context.WithoutCancel(mctx))
	errgrp, ctx := errgroup.WithContext(ctx)
	return &ContractManifestProvider{
		stateCaller: stateCaller,
		address:     config.ParameterContractAddress,
		CheckPeriod: 15 * time.Minute,

		manifestChanges: make(chan *manifest.Manifest, 1),

		errgrp:     errgrp,
		runningCtx: ctx,
		cancel:     cancel,
	}, nil
}

func (cmp *ContractManifestProvider) Start(context.Context) error {
	// no address, nothing to do
	if len(cmp.address) == 0 {
		return nil
	}

	cmp.errgrp.Go(func() error {
		t := time.NewTicker(cmp.CheckPeriod)
		defer t.Stop()

		var knownManifest *manifest.Manifest
	loop:
		for cmp.runningCtx.Err() == nil {
			select {
			case <-t.C:
				m, err := cmp.fetchActivationInfo(cmp.runningCtx)
				if err != nil {
					log.Warnw("got error while fetching manifest from contract", "error", err)
					continue loop
				}

				if knownManifest.Equal(m) {
					continue loop
				}

				if m != nil {
					log.Infow("new manifest from contract", "enable", true, "bootstrapEpoch", m.BootstrapEpoch)
				} else {
					log.Info("new manifest from contract", "enable", false)
				}
				cmp.manifestChanges < -m
				knownManifest = m
			case <-cmp.runningCtx.Done():
			}
		}

		return nil
	})
	return nil
}

func (cmp *ContractManifestProvider) fetchActivationInfo(ctx context.Context) (*manifest.Manifest, error) {
	ethReturn, err := cmp.callContract(ctx)
	if err != nil {
		return nil, fmt.Errorf("calling contract at %s: %w", cmp.address, err)
	}
	if len(ethReturn) == 0 {
		return nil, nil
	}

	activationEpoch, compressedManifest, err := cmp.parseContractReturn(ethReturn)

	if activationEpoch == math.MaxUint64 || len(compressedManifest) == 0 {
		return nil, nil
	}

	reader := io.LimitReader(flate.NewReader(bytes.NewReader(compressedManifest)), 1<<20)
	var m manifest.Manifest
	err = json.NewDecoder(reader).Decode(&m)
	if err != nil {
		return nil, fmt.Errorf("got error while decoding manifest: %w", err)
	}

	if m.BootstrapEpoch < 0 || uint64(m.BootstrapEpoch) != activationEpoch {
		return nil, fmt.Errorf("bootstrap epoch does not match: %d != %d", m.BootstrapEpoch, activationEpoch)
	}

	// TODO: consider addtional checks

	return &m, nil
}

func (cmp *ContractManifestProvider) callContract(ctx context.Context) ([]byte, error) {
	address, err := ethtypes.ParseEthAddress(cmp.address)
	if err != nil {
		return nil, fmt.Errorf("trying to parse contract address: %s: %w", cmp.address, err)
	}

	ethCall := ethtypes.EthCall{
		To:   &address,
		Data: must.One(ethtypes.DecodeHexString("0x2587660d")), // method ID of activationInformation()
	}

	fMessage, err := ethCall.ToFilecoinMessage()
	if err != nil {
		return nil, fmt.Errorf("converting to filecoin message: %w", err)
	}

	msgRes, err := cmp.stateCaller.StateCall(ctx, fMessage, types.EmptyTSK)
	if err != nil {
		return nil, fmt.Errorf("state call error: %w", err)
	}
	if msgRes.MsgRct.ExitCode != 0 {
		return nil, fmt.Errorf("message returned exit code %v: %v", msgRes.MsgRct.ExitCode, msgRes.Error)
	}

	var ethReturn abi.CborBytes
	err = ethReturn.UnmarshalCBOR(bytes.NewReader(msgRes.MsgRct.Return))
	if err != nil {
		return nil, fmt.Errorf("could not decode return value: %w", err)
	}
	return []byte(ethReturn), nil
}

func (cmp *ContractManifestProvider) parseContractReturn(retBytes []byte) (uint64, []byte, error) {
	// 3*32 because there should be 3 slots minimum
	if len(retBytes) < 3*32 {
		return 0, nil, fmt.Errorf("no activation infromation")
	}

	var slot []byte
	// split off first slot
	slot, retBytes = retBytes[:32], retBytes[32:]
	// it is uint64 so we want the last 8 bytes
	slot = slot[24:32]
	activationEpoch := binary.BigEndian.Uint64(slot)

	// next slot is the offest to variable length bytes
	// it is always the same 0x00000...0040
	slot, retBytes = retBytes[:32], retBytes[32:]
	for i := 0; i < 31; i++ {
		if slot[i] != 0 {
			return 0, nil, fmt.Errorf("wrong value for offest (padding): slot[%d] = 0x%x != 0x00", i, slot[i])
		}
	}
	if slot[31] != 0x40 {
		return 0, nil, fmt.Errorf("wrong value for offest : slot[31] = 0x%x != 0x40", slot[31])
	}

	// finally after that there are manifest bytes
	slot, retBytes = retBytes[:32], retBytes[32:]
	slot = slot[24:32]
	pLen := binary.BigEndian.Uint64(slot)
	if pLen > 4<<10 {
		return 0, nil, fmt.Errorf("too long declared payload: %d > %d", pLen, 4<<10)
	}
	payloadLength := int(pLen)

	if payloadLength > len(retBytes) {
		return 0, nil, fmt.Errorf("not enough remaining bytes: %d > %d", payloadLength, retBytes)
	}

	return activationEpoch, retBytes[:payloadLength], nil
}

func (cmp *ContractManifestProvider) Stop(context.Context) error {
	cmp.cancel()
	return cmp.errgrp.Wait()
}

func (cmp *ContractManifestProvider) ManifestUpdates() <-chan *manifest.Manifest {
	return cmp.manifestChanges
}
