// stm: #integration
package itests

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	blocks "github.com/ipfs/go-libipfs/blocks"
	"github.com/ipld/go-car"
	textselector "github.com/ipld/go-ipld-selector-text-lite"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	api0 "github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/itests/kit"
)

// please talk to @ribasushi or @mikeal before modifying these test: there are
// downstream dependencies on ADL-less operation
var (
	adlFixtureCar           = "fixtures/adl_test.car"
	adlFixtureRoot, _       = cid.Parse("bafybeiaigxwanoxyeuzyiknhrg6io6kobfbm37ozcips6qdwumub2gaomy")
	adlFixtureCommp, _      = cid.Parse("baga6ea4seaqjnmnrv4qsfz2rnda54mvo5al22dwpguhn2pmep63gl7bbqqqraai")
	adlFixturePieceSize     = abi.PaddedPieceSize(1024)
	dmSelector              = api.Selector("Links/0/Hash")
	dmTextSelector          = textselector.Expression(dmSelector)
	dmExpectedResult        = "NO ADL"
	dmExpectedCarBlockCount = 4
	dmDagSpec               = []api.DagSpec{{DataSelector: &dmSelector, ExportMerkleProof: true}}
)

func TestDMLevelPartialRetrieval(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001
	//stm: @CLIENT_RETRIEVAL_RETRIEVE_001, @CLIENT_RETRIEVAL_FIND_001
	ctx := context.Background()

	kit.QuietMiningLogs()
	client, miner, ens := kit.EnsembleMinimal(t, kit.ThroughRPC(), kit.MockProofs())
	dh := kit.NewDealHarness(t, client, miner, miner)
	ens.InterconnectAll().BeginMining(50 * time.Millisecond)

	_, err := client.ClientImport(ctx, api.FileRef{Path: adlFixtureCar, IsCAR: true})
	require.NoError(t, err)

	caddr, err := client.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	//
	// test retrieval from local car 1st
	require.NoError(t, testDMExportAsCar(
		ctx, client, api.ExportRef{
			FromLocalCAR: adlFixtureCar,
			Root:         adlFixtureRoot,
			DAGs:         dmDagSpec,
		}, t.TempDir(),
	))
	require.NoError(t, testDMExportAsFile(
		ctx, client, api.ExportRef{
			FromLocalCAR: adlFixtureCar,
			Root:         adlFixtureRoot,
			DAGs:         dmDagSpec,
		}, t.TempDir(),
	))

	//
	// ensure V0 continues functioning as expected
	require.NoError(t, tesV0RetrievalAsCar(
		ctx, client, api0.RetrievalOrder{
			FromLocalCAR:          adlFixtureCar,
			Root:                  adlFixtureRoot,
			DatamodelPathSelector: &dmTextSelector,
		}, t.TempDir(),
	))
	require.NoError(t, testV0RetrievalAsFile(
		ctx, client, api0.RetrievalOrder{
			FromLocalCAR:          adlFixtureCar,
			Root:                  adlFixtureRoot,
			DatamodelPathSelector: &dmTextSelector,
		}, t.TempDir(),
	))

	//
	// now perform a storage/retrieval deal as well, and retest
	dp := dh.DefaultStartDealParams()
	dp.Data = &storagemarket.DataRef{
		Root:      adlFixtureRoot,
		PieceCid:  &adlFixtureCommp,
		PieceSize: adlFixturePieceSize.Unpadded(),
	}
	proposalCid := dh.StartDeal(ctx, dp)

	// Wait for the deal to reach StorageDealCheckForAcceptance on the client
	cd, err := client.ClientGetDealInfo(ctx, *proposalCid)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		cd, _ := client.ClientGetDealInfo(ctx, *proposalCid)
		return cd.State == storagemarket.StorageDealCheckForAcceptance
	}, 30*time.Second, 1*time.Second, "actual deal status is %s", storagemarket.DealStates[cd.State])

	dh.WaitDealSealed(ctx, proposalCid, false, false, nil)

	offers, err := client.ClientFindData(ctx, adlFixtureRoot, nil)
	require.NoError(t, err)
	require.NotEmpty(t, offers, "no offers")

	retOrder := offers[0].Order(caddr)
	retOrder.DataSelector = &dmSelector

	rr, err := client.ClientRetrieve(ctx, retOrder)
	require.NoError(t, err)

	err = client.ClientRetrieveWait(ctx, rr.DealID)
	require.NoError(t, err)

	require.NoError(t, testDMExportAsCar(
		ctx, client, api.ExportRef{
			DealID: rr.DealID,
			Root:   adlFixtureRoot,
			DAGs:   dmDagSpec,
		}, t.TempDir(),
	))
	require.NoError(t, testDMExportAsFile(
		ctx, client, api.ExportRef{
			DealID: rr.DealID,
			Root:   adlFixtureRoot,
			DAGs:   dmDagSpec,
		}, t.TempDir(),
	))

}

func testDMExportAsFile(ctx context.Context, client *kit.TestFullNode, expDirective api.ExportRef, tempDir string) error {
	out := tempDir + string(os.PathSeparator) + "exp-test" + expDirective.Root.String()

	fileDest := api.FileRef{
		Path: out,
	}
	err := client.ClientExport(ctx, expDirective, fileDest)
	if err != nil {
		return err
	}

	f, err := os.Open(out)
	if err != nil {
		return err
	}

	defer f.Close() //nolint:errcheck

	return validateDMUnixFile(f)
}
func testV0RetrievalAsFile(ctx context.Context, client *kit.TestFullNode, retOrder api0.RetrievalOrder, tempDir string) error {
	out := tempDir + string(os.PathSeparator) + "exp-test" + retOrder.Root.String()

	cv0 := &api0.WrapperV1Full{FullNode: client.FullNode}
	err := cv0.ClientRetrieve(ctx, retOrder, &api.FileRef{
		Path: out,
	})
	if err != nil {
		return err
	}

	f, err := os.Open(out)
	if err != nil {
		return err
	}

	defer f.Close() //nolint:errcheck

	return validateDMUnixFile(f)
}
func validateDMUnixFile(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if string(data) != dmExpectedResult {
		return fmt.Errorf("retrieved data mismatch: expected '%s' got '%s'", dmExpectedResult, data)
	}

	return nil
}

func testDMExportAsCar(ctx context.Context, client *kit.TestFullNode, expDirective api.ExportRef, tempDir string) error {
	out, err := ioutil.TempFile(tempDir, "exp-test")
	if err != nil {
		return err
	}
	defer out.Close() //nolint:errcheck

	carDest := api.FileRef{
		IsCAR: true,
		Path:  out.Name(),
	}
	err = client.ClientExport(ctx, expDirective, carDest)
	if err != nil {
		return err
	}

	return validateDMCar(out)
}
func tesV0RetrievalAsCar(ctx context.Context, client *kit.TestFullNode, retOrder api0.RetrievalOrder, tempDir string) error {
	out, err := ioutil.TempFile(tempDir, "exp-test")
	if err != nil {
		return err
	}
	defer out.Close() //nolint:errcheck

	cv0 := &api0.WrapperV1Full{FullNode: client.FullNode}
	err = cv0.ClientRetrieve(ctx, retOrder, &api.FileRef{
		Path:  out.Name(),
		IsCAR: true,
	})
	if err != nil {
		return err
	}

	return validateDMCar(out)
}
func validateDMCar(r io.Reader) error {
	cr, err := car.NewCarReader(r)
	if err != nil {
		return err
	}

	if len(cr.Header.Roots) != 1 {
		return fmt.Errorf("expected a single root in result car, got %d", len(cr.Header.Roots))
	} else if cr.Header.Roots[0].String() != adlFixtureRoot.String() {
		return fmt.Errorf("expected root cid '%s', got '%s'", adlFixtureRoot.String(), cr.Header.Roots[0].String())
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

	if len(blks) != dmExpectedCarBlockCount {
		return fmt.Errorf("expected a car file with %d blocks, got one with %d instead", dmExpectedCarBlockCount, len(blks))
	}

	data := fmt.Sprintf("%s%s", blks[2].RawData(), blks[3].RawData())
	if data != dmExpectedResult {
		return fmt.Errorf("retrieved data mismatch: expected '%s' got '%s'", dmExpectedResult, data)
	}

	return nil
}
