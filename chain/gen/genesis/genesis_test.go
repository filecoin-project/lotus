package genesis_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	genesisbuilder "github.com/filecoin-project/lotus/chain/gen/genesis"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/genesis"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
)

func TestInitialStateTreeFilplusRoot(t *testing.T) {
	rootOwner, err := address.NewSecp256k1Address([]byte("filplus root"))
	require.NoError(t, err)
	accountOwner, err := address.NewSecp256k1Address([]byte("genesis account"))
	require.NoError(t, err)
	remainderOwner, err := address.NewSecp256k1Address([]byte("genesis remainder"))
	require.NoError(t, err)

	account := func(owner address.Address, balance int64) genesis.Actor {
		meta := genesis.AccountMeta{Owner: owner}
		return genesis.Actor{Type: genesis.TAccount, Balance: abi.NewTokenAmount(balance), Meta: meta.ActorMeta()}
	}
	rootSigners := genesis.MultisigMeta{Signers: []address.Address{rootOwner, accountOwner}, Threshold: 1}

	for _, tc := range []struct {
		name string
		nv   network.Version
		root genesis.Actor
	}{
		{name: "nv30 omitted", nv: network.Version30},
		{name: "nv30 account", nv: network.Version30, root: account(rootOwner, 10)},
		{name: "nv30 multisig", nv: network.Version30, root: genesis.Actor{
			Type: genesis.TMultisig, Balance: abi.NewTokenAmount(10), Meta: rootSigners.ActorMeta(),
		}},
		{name: "nv29 account", nv: network.Version29, root: account(rootOwner, 10)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, _, err := genesisbuilder.MakeInitialStateTree(t.Context(), blockstore.NewMemorySync(), genesis.Template{
				NetworkVersion:   tc.nv,
				NetworkName:      "filplus-root-test",
				Accounts:         []genesis.Actor{account(accountOwner, 5)},
				VerifregRootKey:  tc.root,
				RemainderAccount: account(remainderOwner, 0),
			})
			require.NoError(t, err)

			for _, id := range []uint64{6, 7, 80, 81} {
				addr, err := address.NewIDAddress(id)
				require.NoError(t, err)
				_, err = st.GetActor(addr)
				if tc.nv >= network.Version30 {
					require.ErrorIs(t, err, types.ErrActorNotFound, "actor %s", addr)
				} else {
					require.NoError(t, err, "actor %s", addr)
				}
			}

			rootID, err := st.LookupIDAddress(rootOwner)
			if tc.nv >= network.Version30 {
				require.ErrorIs(t, err, types.ErrActorNotFound)
			} else {
				require.NoError(t, err)
				id, err := address.IDFromAddress(rootID)
				require.NoError(t, err)
				require.EqualValues(t, 80, id)
			}

			act, err := st.GetActor(accountOwner)
			require.NoError(t, err)
			require.Equal(t, "5", act.Balance.String())

			total := big.Zero()
			require.NoError(t, st.ForEach(func(_ address.Address, act *types.Actor) error {
				total = big.Add(total, act.Balance)
				return nil
			}))
			expected := big.Mul(big.NewIntUnsigned(buildconstants.FilBase), big.NewIntUnsigned(buildconstants.FilecoinPrecision))
			require.Equal(t, expected.String(), total.String())
		})
	}
}
