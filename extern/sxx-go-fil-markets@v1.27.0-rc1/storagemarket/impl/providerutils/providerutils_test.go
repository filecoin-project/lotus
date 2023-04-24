package providerutils_test

import (
	"bytes"
	"context"
	"errors"
	"math/rand"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/blockrecorder"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerutils"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
)

func TestVerifyProposal(t *testing.T) {
	tests := map[string]struct {
		proposal  market.ClientDealProposal
		verifier  providerutils.VerifyFunc
		shouldErr bool
	}{
		"successful verification": {
			proposal: *shared_testutil.MakeTestClientDealProposal(),
			verifier: func(context.Context, crypto.Signature, address.Address, []byte, shared.TipSetToken) (bool, error) {
				return true, nil
			},
			shouldErr: false,
		},
		"bad proposal": {
			proposal: market.ClientDealProposal{
				Proposal:        market.DealProposal{},
				ClientSignature: *shared_testutil.MakeTestSignature(),
			},
			verifier: func(context.Context, crypto.Signature, address.Address, []byte, shared.TipSetToken) (bool, error) {
				return true, nil
			},
			shouldErr: true,
		},
		"verification fails": {
			proposal: *shared_testutil.MakeTestClientDealProposal(),
			verifier: func(context.Context, crypto.Signature, address.Address, []byte, shared.TipSetToken) (bool, error) {
				return false, nil
			},
			shouldErr: true,
		},
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			err := providerutils.VerifyProposal(context.Background(), data.proposal, shared.TipSetToken{}, data.verifier)
			require.Equal(t, err != nil, data.shouldErr)
		})
	}
}

func TestSignMinerData(t *testing.T) {
	ctx := context.Background()
	successLookup := func(context.Context, address.Address, shared.TipSetToken) (address.Address, error) {
		return address.TestAddress2, nil
	}
	successSign := func(context.Context, address.Address, []byte) (*crypto.Signature, error) {
		return shared_testutil.MakeTestSignature(), nil
	}
	tests := map[string]struct {
		data         interface{}
		workerLookup providerutils.WorkerLookupFunc
		signBytes    providerutils.SignFunc
		shouldErr    bool
	}{
		"succeeds": {
			data:         shared_testutil.MakeTestStorageAsk(),
			workerLookup: successLookup,
			signBytes:    successSign,
			shouldErr:    false,
		},
		"cbor dump errors": {
			data:         &network.Response{},
			workerLookup: successLookup,
			signBytes:    successSign,
			shouldErr:    true,
		},
		"worker lookup errors": {
			data: shared_testutil.MakeTestStorageAsk(),
			workerLookup: func(context.Context, address.Address, shared.TipSetToken) (address.Address, error) {
				return address.Undef, errors.New("Something went wrong")
			},
			signBytes: successSign,
			shouldErr: true,
		},
		"signing errors": {
			data:         shared_testutil.MakeTestStorageAsk(),
			workerLookup: successLookup,
			signBytes: func(context.Context, address.Address, []byte) (*crypto.Signature, error) {
				return nil, errors.New("something went wrong")
			},
			shouldErr: true,
		},
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := providerutils.SignMinerData(ctx, data.data, address.TestAddress, shared.TipSetToken{}, data.workerLookup, data.signBytes)
			require.Equal(t, err != nil, data.shouldErr)
		})
	}
}

func TestLoadBlockLocations(t *testing.T) {
	testData := shared_testutil.NewTestIPLDTree()

	carBuf := new(bytes.Buffer)
	blockLocationBuf := new(bytes.Buffer)
	err := testData.DumpToCar(carBuf, blockrecorder.RecordEachBlockTo(blockLocationBuf))
	require.NoError(t, err)
	validPath := filestore.Path("valid.data")
	validFile := shared_testutil.NewTestFile(shared_testutil.TestFileParams{
		Buffer: blockLocationBuf,
		Path:   validPath,
	})
	missingPath := filestore.Path("missing.data")
	invalidPath := filestore.Path("invalid.data")
	invalidData := make([]byte, 512)
	_, _ = rand.Read(invalidData)
	invalidFile := shared_testutil.NewTestFile(shared_testutil.TestFileParams{
		Buffer: bytes.NewBuffer(invalidData),
		Path:   invalidPath,
	})
	fs := shared_testutil.NewTestFileStore(shared_testutil.TestFileStoreParams{
		Files:         []filestore.File{validFile, invalidFile},
		ExpectedOpens: []filestore.Path{validPath, invalidPath},
	})
	testCases := map[string]struct {
		path         filestore.Path
		shouldErr    bool
		expectedCids []cid.Cid
	}{
		"valid data": {
			path:      validPath,
			shouldErr: false,
			expectedCids: []cid.Cid{
				testData.LeafAlphaBlock.Cid(),
				testData.LeafBetaBlock.Cid(),
				testData.MiddleListBlock.Cid(),
				testData.MiddleMapBlock.Cid(),
				testData.RootBlock.Cid(),
			},
		},
		"missing data": {
			path:      missingPath,
			shouldErr: true,
		},
		"invalid data": {
			path:      invalidPath,
			shouldErr: true,
		},
	}
	for testCase, data := range testCases {
		t.Run(testCase, func(t *testing.T) {
			results, err := providerutils.LoadBlockLocations(fs, data.path)
			if data.shouldErr {
				require.Error(t, err)
				require.Nil(t, results)
			} else {
				require.NoError(t, err)
				for _, c := range data.expectedCids {
					_, ok := results[c]
					require.True(t, ok)
				}
			}
		})
	}
	fs.VerifyExpectations(t)
}
