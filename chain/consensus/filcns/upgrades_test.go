package filcns

import (
	"context"
	"testing"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	nv29 "github.com/filecoin-project/go-state-types/builtin/v19/migration"
	reward19 "github.com/filecoin-project/go-state-types/builtin/v19/reward"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
)

func TestSolsticeRewardMigrationConfig(t *testing.T) {
	t.Run("neutral bootstrap", func(t *testing.T) {
		params := buildconstants.NeutralSolsticeRewardBootstrapParams
		params.SWATimelockEpochs = 42

		config, err := solsticeRewardMigrationConfig(params)
		require.NoError(t, err)
		require.Equal(t, nv29.RewardMigrationConfig{
			SWATimelockEpochs: 42,
			SWAActor:          builtin.SystemActorAddr,
			Streams: []nv29.RewardMigrationStream{{
				ID: 1,
				Weight: nv29.RewardMigrationWeight{
					VStart: reward19.Denom,
					Floor:  reward19.Denom,
					Cap:    reward19.Denom,
				},
			}},
		}, config)
	})

	t.Run("zero duration rejects non-neutral consensus weight", func(t *testing.T) {
		params := buildconstants.NeutralSolsticeRewardBootstrapParams
		params.ConsensusWeight.Cap--

		_, err := solsticeRewardMigrationConfig(params)
		require.EqualError(t, err, "zero-duration Solstice bootstrap must have constant DENOM consensus weight and zero service weight")
	})

	t.Run("zero duration rejects service weight", func(t *testing.T) {
		params := buildconstants.NeutralSolsticeRewardBootstrapParams
		params.ServiceWeight.VStart = 1

		_, err := solsticeRewardMigrationConfig(params)
		require.EqualError(t, err, "zero-duration Solstice bootstrap must have constant DENOM consensus weight and zero service weight")
	})

	t.Run("negative duration", func(t *testing.T) {
		params := buildconstants.NeutralSolsticeRewardBootstrapParams
		params.ConsensusWeightRampDurationEpochs = -1

		_, err := solsticeRewardMigrationConfig(params)
		require.EqualError(t, err, "Solstice consensus weight ramp duration is negative: -1")
	})

	t.Run("split bootstrap", func(t *testing.T) {
		pct := reward19.Denom / 100
		params := buildconstants.SolsticeRewardBootstrapParams{
			SWATimelockEpochs:                 8,
			ConsensusWeightRampDurationEpochs: 81,
			ConsensusWeight: buildconstants.SolsticeRewardWeightParams{
				VStart: 95 * pct,
				Floor:  50 * pct,
				Cap:    95 * pct,
			},
			ServiceWeight: buildconstants.SolsticeRewardWeightParams{
				VStart: 5 * pct,
				Floor:  5 * pct,
				Cap:    10 * pct,
			},
			SWAActor:            builtin.SystemActorAddr,
			SRAActor:            builtin.SystemActorAddr,
			InitialOrchestrator: builtin.SystemActorAddr,
		}
		rampTotal := params.ConsensusWeight.VStart - params.ConsensusWeight.Floor
		rampEpochs := uint64(params.ConsensusWeightRampDurationEpochs)
		slope := rampTotal / rampEpochs
		if rampTotal%rampEpochs != 0 {
			slope++
		}

		config, err := solsticeRewardMigrationConfig(params)
		require.NoError(t, err)
		require.Equal(t, nv29.RewardMigrationConfig{
			SWATimelockEpochs: params.SWATimelockEpochs,
			SWAActor:          params.SWAActor,
			Streams: []nv29.RewardMigrationStream{
				{
					ID: 1,
					Weight: nv29.RewardMigrationWeight{
						VStart: params.ConsensusWeight.VStart,
						Slope:  -int64(slope),
						Floor:  params.ConsensusWeight.Floor,
						Cap:    params.ConsensusWeight.Cap,
					},
				},
				{
					ID: 2,
					Weight: nv29.RewardMigrationWeight{
						VStart: params.ServiceWeight.VStart,
						Slope:  int64(slope),
						Floor:  params.ServiceWeight.Floor,
						Cap:    params.ServiceWeight.Cap,
					},
					Distribution: &reward19.DistributionInit{
						Writer: params.SRAActor,
						Shares: []reward19.RecipientShare{{
							Recipient: params.InitialOrchestrator,
							Share:     reward19.Denom,
						}},
					},
				},
			},
		}, config)
	})
}

// The Solstice bootstrap names contracts that must exist before the upgrade runs, so a scheduled
// height needs real addresses. Addresses may be baked ahead of a height. Checks the network being
// built.
func TestSolsticeBootstrapMatchesSchedule(t *testing.T) {
	params := buildconstants.UpgradeSolsticeRewardBootstrapParams

	addressesSet := params.SWAActor != address.Undef &&
		params.SRAActor != address.Undef &&
		params.InitialOrchestrator != address.Undef
	scheduled := buildconstants.UpgradeSolsticeHeight < buildconstants.UpgradeHeightUnscheduled

	if !scheduled {
		return
	}

	require.True(t, addressesSet,
		"Solstice is scheduled at epoch %d, so SWAActor (%v), SRAActor (%v) and "+
			"InitialOrchestrator (%v) must all be set.",
		buildconstants.UpgradeSolsticeHeight,
		params.SWAActor, params.SRAActor, params.InitialOrchestrator)

	// The migration resolves the addresses against its input state tree, so stand in ID addresses
	// here and leave the weight geometry as the part a build-time check can reach.
	params.SWAActor = builtin.SystemActorAddr
	params.SRAActor = builtin.SystemActorAddr
	params.InitialOrchestrator = builtin.BurntFundsActorAddr

	config, err := solsticeRewardMigrationConfig(params)
	require.NoError(t, err)
	require.NoError(t, nv29.ValidateRewardMigrationConfig(config, buildconstants.UpgradeSolsticeHeight),
		"Solstice is scheduled at epoch %d, so its reward bootstrap must be valid",
		buildconstants.UpgradeSolsticeHeight)
}

func TestResolveSolsticeRewardBootstrap(t *testing.T) {
	swa, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, ethAddressBytes(1))
	require.NoError(t, err)
	sra, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, ethAddressBytes(2))
	require.NoError(t, err)
	orchestrator, err := address.NewSecp256k1Address([]byte("solstice orchestrator"))
	require.NoError(t, err)

	deployed := func() buildconstants.SolsticeRewardBootstrapParams {
		params := buildconstants.NeutralSolsticeRewardBootstrapParams
		params.SWAActor = swa
		params.SRAActor = sra
		params.InitialOrchestrator = orchestrator
		return params
	}

	t.Run("resolves f410 and f1 addresses", func(t *testing.T) {
		tree, ids := solsticeStateTree(t, swa, sra, orchestrator)

		resolved, err := resolveSolsticeRewardBootstrap(tree, deployed())
		require.NoError(t, err)
		require.Equal(t, ids[swa], resolved.SWAActor)
		require.Equal(t, ids[sra], resolved.SRAActor)
		require.Equal(t, ids[orchestrator], resolved.InitialOrchestrator)
		for _, addr := range []address.Address{resolved.SWAActor, resolved.SRAActor, resolved.InitialOrchestrator} {
			require.Equal(t, address.ID, addr.Protocol())
		}
	})

	t.Run("ID addresses pass through", func(t *testing.T) {
		tree, _ := solsticeStateTree(t)
		params := buildconstants.NeutralSolsticeRewardBootstrapParams
		params.SWAActor = builtin.SystemActorAddr
		params.SRAActor = builtin.SystemActorAddr
		params.InitialOrchestrator = builtin.SystemActorAddr

		resolved, err := resolveSolsticeRewardBootstrap(tree, params)
		require.NoError(t, err)
		require.Equal(t, params, resolved)
	})

	t.Run("unresolvable address fails", func(t *testing.T) {
		for _, missing := range []struct {
			field string
			addr  address.Address
		}{
			{"SWAActor", swa},
			{"SRAActor", sra},
			{"InitialOrchestrator", orchestrator},
		} {
			t.Run(missing.field, func(t *testing.T) {
				var registered []address.Address
				for _, addr := range []address.Address{swa, sra, orchestrator} {
					if addr != missing.addr {
						registered = append(registered, addr)
					}
				}
				tree, _ := solsticeStateTree(t, registered...)

				_, err := resolveSolsticeRewardBootstrap(tree, deployed())
				require.ErrorIs(t, err, types.ErrActorNotFound)
				require.ErrorContains(t, err, missing.field)
				require.ErrorContains(t, err, missing.addr.String())
			})
		}
	})

	t.Run("unset SWA fails", func(t *testing.T) {
		tree, _ := solsticeStateTree(t, sra, orchestrator)
		params := deployed()
		params.SWAActor = address.Undef

		_, err := resolveSolsticeRewardBootstrap(tree, params)
		require.ErrorContains(t, err, "SWAActor is unset")
	})

	t.Run("burn actor as orchestrator fails", func(t *testing.T) {
		tree, _ := solsticeStateTree(t, swa, sra)
		params := deployed()
		params.InitialOrchestrator = builtin.BurntFundsActorAddr

		_, err := resolveSolsticeRewardBootstrap(tree, params)
		require.ErrorContains(t, err, "InitialOrchestrator is the burn actor")
	})

	t.Run("unset SRA and orchestrator pass through", func(t *testing.T) {
		tree, ids := solsticeStateTree(t, swa)
		params := deployed()
		params.SRAActor = address.Undef
		params.InitialOrchestrator = address.Undef

		resolved, err := resolveSolsticeRewardBootstrap(tree, params)
		require.NoError(t, err)
		require.Equal(t, ids[swa], resolved.SWAActor)
		require.Equal(t, address.Undef, resolved.SRAActor)
		require.Equal(t, address.Undef, resolved.InitialOrchestrator)
	})
}

func ethAddressBytes(b byte) []byte {
	addr := make([]byte, 20)
	addr[19] = b
	return addr
}

// solsticeStateTree returns a state tree whose init actor maps each address to a fresh ID address,
// along with those mappings.
func solsticeStateTree(
	t *testing.T, addrs ...address.Address,
) (*state.StateTree, map[address.Address]address.Address) {
	t.Helper()

	ctx := context.Background()
	cst := cbor.NewMemCborStore()
	tree, err := state.NewStateTree(cst, types.StateTreeVersion5)
	require.NoError(t, err)

	initState, err := init_.MakeState(adt.WrapStore(ctx, cst), actorstypes.Version18, "testnet")
	require.NoError(t, err)
	head, err := cst.Put(ctx, initState)
	require.NoError(t, err)
	require.NoError(t, tree.SetActor(init_.Address, &types.Actor{
		Code:    initState.Code(),
		Head:    head,
		Balance: big.Zero(),
	}))

	ids := make(map[address.Address]address.Address, len(addrs))
	for _, addr := range addrs {
		id, err := tree.RegisterNewAddress(addr)
		require.NoError(t, err)
		ids[addr] = id
	}
	return tree, ids
}
