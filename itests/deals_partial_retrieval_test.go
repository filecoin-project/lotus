package itests

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/itests/kit"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car"
	"github.com/stretchr/testify/require"
)

// use the mainnet carfile as text fixture: it will always be here
// https://dweb.link/ipfs/bafy2bzacecnamqgqmifpluoeldx7zzglxcljo6oja4vrmtj7432rphldpdmm2/8/1/8/1/0/1/0
var (
	sourceCar               = "../build/genesis/mainnet.car"
	carRoot, _              = cid.Parse("bafy2bzacecnamqgqmifpluoeldx7zzglxcljo6oja4vrmtj7432rphldpdmm2")
	carCommp, _             = cid.Parse("baga6ea4seaqmrivgzei3fmx5qxtppwankmtou6zvigyjaveu3z2zzwhysgzuina")
	selectedCid, _          = cid.Parse("bafkqaetgnfwc6mjpon2g64tbm5sxa33xmvza")
	carPieceSize            = abi.PaddedPieceSize(2097152)
	textSelector            = api.Selector("8/1/8/1/0/1/0")
	textSelectorNonLink     = api.Selector("8/1/8/1/0/1")
	textSelectorNonexistent = api.Selector("42")
	expectedResult          = "fil/1/storagepower"
)

func TestPartialRetrieval(t *testing.T) {

	ctx := context.Background()

	policy.SetPreCommitChallengeDelay(2)
	kit.EnableLargeSectors(t)
	kit.QuietMiningLogs()
	client, miner, ens := kit.EnsembleMinimal(t, kit.ThroughRPC(), kit.MockProofs(), kit.SectorSize(512<<20))
	dh := kit.NewDealHarness(t, client, miner, miner)
	ens.InterconnectAll().BeginMining(50 * time.Millisecond)

	_, err := client.ClientImport(ctx, api.FileRef{Path: sourceCar, IsCAR: true})
	require.NoError(t, err)

	caddr, err := client.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	// first test retrieval from local car, then do an actual deal
	for _, exportMerkleProof := range []bool{false, true} {
		for _, fullCycle := range []bool{false, true} {

			var retOrder api.RetrievalOrder
			var eref api.ExportRef

			if !fullCycle {
				eref.FromLocalCAR = sourceCar
			} else {
				dp := dh.DefaultStartDealParams()
				dp.Data = &storagemarket.DataRef{
					// FIXME: figure out how to do this with an online partial transfer
					TransferType: storagemarket.TTManual,
					Root:         carRoot,
					PieceCid:     &carCommp,
					PieceSize:    carPieceSize.Unpadded(),
				}
				proposalCid := dh.StartDeal(ctx, dp)

				// Wait for the deal to reach StorageDealCheckForAcceptance on the client
				cd, err := client.ClientGetDealInfo(ctx, *proposalCid)
				require.NoError(t, err)
				require.Eventually(t, func() bool {
					cd, _ := client.ClientGetDealInfo(ctx, *proposalCid)
					return cd.State == storagemarket.StorageDealCheckForAcceptance
				}, 30*time.Second, 1*time.Second, "actual deal status is %s", storagemarket.DealStates[cd.State])

				err = miner.DealsImportData(ctx, *proposalCid, sourceCar)
				require.NoError(t, err)

				// Wait for the deal to be published, we should be able to start retrieval right away
				dh.WaitDealPublished(ctx, proposalCid)

				offers, err := client.ClientFindData(ctx, carRoot, nil)
				require.NoError(t, err)
				require.NotEmpty(t, offers, "no offers")

				retOrder = offers[0].Order(caddr)
			}

			retOrder.DataSelector = &textSelector
			eref.DAGs = append(eref.DAGs, api.DagSpec{
				DataSelector:      &textSelector,
				ExportMerkleProof: exportMerkleProof,
			})
			eref.Root = carRoot

			// test retrieval of either data or constructing a partial selective-car
			for _, retrieveAsCar := range []bool{false, true} {
				outFile, err := ioutil.TempFile(t.TempDir(), "ret-file")
				require.NoError(t, err)
				defer outFile.Close() //nolint:errcheck

				require.NoError(t, testGenesisRetrieval(
					ctx,
					client,
					retOrder,
					eref,
					&api.FileRef{
						Path:  outFile.Name(),
						IsCAR: retrieveAsCar,
					},
					outFile,
				))

				// UGH if I do not sleep here, I get things like:
				/*
					retrieval failed: Retrieve failed: there is an active retrieval deal with peer 12D3KooWK9fB9a3HZ4PQLVmEQ6pweMMn5CAyKtumB71CPTnuBDi6 for payload CID bafy2bzacecnamqgqmifpluoeldx7zzglxcljo6oja4vrmtj7432rphldpdmm2 (retrieval deal ID 1631259332180384709, state DealStatusFinalizingBlockstore) - existing deal must be cancelled before starting a new retrieval deal:
						github.com/filecoin-project/lotus/node/impl/client.(*API).ClientRetrieve
							/home/circleci/project/node/impl/client/client.go:774
				*/
				time.Sleep(time.Second)
			}
		}
	}

	// ensure non-existent paths fail
	require.EqualError(
		t,
		testGenesisRetrieval(
			ctx,
			client,
			api.RetrievalOrder{
				Root:         carRoot,
				DataSelector: &textSelectorNonexistent,
			},
			api.ExportRef{
				Root:         carRoot,
				FromLocalCAR: sourceCar,
				DAGs:         []api.DagSpec{{DataSelector: &textSelectorNonexistent}},
			},
			&api.FileRef{},
			nil,
		),
		fmt.Sprintf("parsing dag spec: path selection does not match a node within %s", carRoot),
	)

	// ensure non-boundary retrievals fail
	require.EqualError(
		t,
		testGenesisRetrieval(
			ctx,
			client,
			api.RetrievalOrder{
				Root:         carRoot,
				DataSelector: &textSelectorNonLink,
			},
			api.ExportRef{
				Root:         carRoot,
				FromLocalCAR: sourceCar,
				DAGs:         []api.DagSpec{{DataSelector: &textSelectorNonLink}},
			},
			&api.FileRef{},
			nil,
		),
		fmt.Sprintf("parsing dag spec: error while locating partial retrieval sub-root: unsupported selection path '%s' does not correspond to a block boundary (a.k.a. CID link)", textSelectorNonLink),
	)
}

func testGenesisRetrieval(ctx context.Context, client *kit.TestFullNode, retOrder api.RetrievalOrder, eref api.ExportRef, retRef *api.FileRef, outFile *os.File) error {

	if retOrder.Total.Nil() {
		retOrder.Total = big.Zero()
	}
	if retOrder.UnsealPrice.Nil() {
		retOrder.UnsealPrice = big.Zero()
	}

	if eref.FromLocalCAR == "" {
		rr, err := client.ClientRetrieve(ctx, retOrder)
		if err != nil {
			return err
		}
		eref.DealID = rr.DealID

		if err := client.ClientRetrieveWait(ctx, rr.DealID); err != nil {
			return xerrors.Errorf("retrieval wait: %w", err)
		}
	}

	err := client.ClientExport(ctx, eref, *retRef)
	if err != nil {
		return err
	}

	var data []byte
	if !retRef.IsCAR {

		data, err = io.ReadAll(outFile)
		if err != nil {
			return err
		}

	} else {

		cr, err := car.NewCarReader(outFile)
		if err != nil {
			return err
		}

		if len(cr.Header.Roots) != 1 {
			return fmt.Errorf("expected a single root in result car, got %d", len(cr.Header.Roots))
		} else if eref.DAGs[0].ExportMerkleProof && cr.Header.Roots[0].String() != carRoot.String() {
			return fmt.Errorf("expected root cid '%s', got '%s'", carRoot.String(), cr.Header.Roots[0].String())
		} else if !eref.DAGs[0].ExportMerkleProof && cr.Header.Roots[0].String() != selectedCid.String() {
			return fmt.Errorf("expected root cid '%s', got '%s'", selectedCid.String(), cr.Header.Roots[0].String())
		}

		blks := make([]blocks.Block, 0)
		for {
			b, err := cr.Next()
			if err == io.EOF {
				break
			} else if err != nil {
				return err
			}

			blks = append(blks, b)
		}

		if (eref.DAGs[0].ExportMerkleProof && len(blks) != 3) || (!eref.DAGs[0].ExportMerkleProof && len(blks) != 1) {
			return fmt.Errorf("expected a car file with 3/1 blocks, got one with %d instead", len(blks))
		}

		data = blks[len(blks)-1].RawData()
	}

	if string(data) != expectedResult {
		return fmt.Errorf("retrieved data mismatch: expected '%s' got '%s'", expectedResult, data)
	}

	return nil
}
