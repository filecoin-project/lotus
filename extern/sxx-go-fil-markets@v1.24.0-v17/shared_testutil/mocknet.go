package shared_testutil

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	dss "github.com/ipfs/go-datastore/sync"
	"github.com/ipfs/go-graphsync/storeutil"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files "github.com/ipfs/go-ipfs-files"
	ipldformat "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/libp2p/go-libp2p-core/host"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"

	dtnet "github.com/filecoin-project/go-data-transfer/network"

	"github.com/filecoin-project/go-fil-markets/shared_testutil/unixfs"
)

type Libp2pTestData struct {
	Ctx         context.Context
	Ds1         datastore.Batching
	Ds2         datastore.Batching
	Bs1         bstore.Blockstore
	Bs2         bstore.Blockstore
	DagService1 ipldformat.DAGService
	DagService2 ipldformat.DAGService
	DTNet1      dtnet.DataTransferNetwork
	DTNet2      dtnet.DataTransferNetwork
	DTStore1    datastore.Batching
	DTStore2    datastore.Batching
	DTTmpDir1   string
	DTTmpDir2   string
	LinkSystem1 ipld.LinkSystem
	LinkSystem2 ipld.LinkSystem
	Host1       host.Host
	Host2       host.Host
	OrigBytes   []byte

	MockNet mocknet.Mocknet
}

func NewLibp2pTestData(ctx context.Context, t *testing.T) *Libp2pTestData {
	testData := &Libp2pTestData{}
	testData.Ctx = ctx

	var err error

	testData.Ds1 = dss.MutexWrap(datastore.NewMapDatastore())
	testData.Ds2 = dss.MutexWrap(datastore.NewMapDatastore())

	// make a bstore and dag service
	testData.Bs1 = bstore.NewBlockstore(testData.Ds1)
	testData.Bs2 = bstore.NewBlockstore(testData.Ds2)

	testData.DagService1 = merkledag.NewDAGService(blockservice.New(testData.Bs1, offline.Exchange(testData.Bs1)))
	testData.DagService2 = merkledag.NewDAGService(blockservice.New(testData.Bs2, offline.Exchange(testData.Bs2)))

	// setup an IPLD link system for bstore 1
	testData.LinkSystem1 = storeutil.LinkSystemForBlockstore(testData.Bs1)

	// setup an IPLD link system for bstore 2
	testData.LinkSystem2 = storeutil.LinkSystemForBlockstore(testData.Bs2)

	mn := mocknet.New()

	// setup network
	testData.Host1, err = mn.GenPeer()
	require.NoError(t, err)

	testData.Host2, err = mn.GenPeer()
	require.NoError(t, err)

	err = mn.LinkAll()
	require.NoError(t, err)

	testData.DTNet1 = dtnet.NewFromLibp2pHost(testData.Host1)
	testData.DTNet2 = dtnet.NewFromLibp2pHost(testData.Host2)

	testData.DTStore1 = namespace.Wrap(testData.Ds1, datastore.NewKey("DataTransfer1"))
	testData.DTStore2 = namespace.Wrap(testData.Ds1, datastore.NewKey("DataTransfer2"))

	testData.DTTmpDir1, err = ioutil.TempDir("", "dt-tmp-1")
	require.NoError(t, err)
	testData.DTTmpDir2, err = ioutil.TempDir("", "dt-tmp-2")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(testData.DTTmpDir1)
		_ = os.RemoveAll(testData.DTTmpDir2)
	})

	testData.MockNet = mn

	return testData
}

// LoadUnixFSFile injects the fixture `src` into the given blockstore from the
// fixtures directory. If useSecondNode is true, fixture is injected to the second node;
// otherwise the first node gets it
func (ltd *Libp2pTestData) LoadUnixFSFile(t *testing.T, src string, useSecondNode bool) (ipld.Link, string) {
	var dagService ipldformat.DAGService
	if useSecondNode {
		dagService = ltd.DagService2
	} else {
		dagService = ltd.DagService1
	}
	return ltd.loadUnixFSFile(t, src, dagService)
}

// LoadUnixFSFileToStore creates a CAR file from the fixture at `src`
func (ltd *Libp2pTestData) LoadUnixFSFileToStore(t *testing.T, src string) (ipld.Link, string) {
	dstore := dss.MutexWrap(datastore.NewMapDatastore())
	bs := bstore.NewBlockstore(dstore)
	dagService := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))

	return ltd.loadUnixFSFile(t, src, dagService)
}

func (ltd *Libp2pTestData) loadUnixFSFile(t *testing.T, src string, dagService ipldformat.DAGService) (ipld.Link, string) {
	f, err := os.Open(src)
	require.NoError(t, err)

	ltd.OrigBytes, err = ioutil.ReadAll(f)
	require.NoError(t, err)
	require.NotEmpty(t, ltd.OrigBytes)

	// generate a unixfs dag using the given dagService to get the root.
	root := unixfs.WriteUnixfsDAGTo(t, src, dagService)

	// Create a UnixFS DAG again AND generate a CARv2 file that can be used to back a filestore.
	path := genRefCARv2(t, src, root)
	return cidlink.Link{Cid: root}, path
}

// VerifyFileTransferred checks that the fixture file was sent from one node to the other.
func (ltd *Libp2pTestData) VerifyFileTransferred(t *testing.T, link ipld.Link, useSecondNode bool, readLen uint64) {
	var dagService ipldformat.DAGService
	if useSecondNode {
		dagService = ltd.DagService2
	} else {
		dagService = ltd.DagService1
	}
	ltd.verifyFileTransferred(t, link, dagService, readLen)
}

// VerifyFileTransferredIntoStore checks that the fixture file was sent from
// one node to the other, and stored in the given CAR file
func (ltd *Libp2pTestData) VerifyFileTransferredIntoStore(t *testing.T, link ipld.Link, bs bstore.Blockstore, readLen uint64) {
	bsvc := blockservice.New(bs, offline.Exchange(bs))
	dagService := merkledag.NewDAGService(bsvc)
	ltd.verifyFileTransferred(t, link, dagService, readLen)
}

func (ltd *Libp2pTestData) verifyFileTransferred(t *testing.T, link ipld.Link, dagService ipldformat.DAGService, readLen uint64) {
	c := link.(cidlink.Link).Cid

	// load the root of the UnixFS DAG from the new blockstore
	otherNode, err := dagService.Get(ltd.Ctx, c)
	require.NoError(t, err)

	// Setup a UnixFS file reader
	n, err := unixfile.NewUnixfsFile(ltd.Ctx, dagService, otherNode)
	require.NoError(t, err)

	fn, ok := n.(files.File)
	require.True(t, ok)

	// Read the bytes for the UnixFS File
	finalBytes := make([]byte, readLen)
	_, err = fn.Read(finalBytes)
	if err != nil {
		require.Equal(t, "EOF", err.Error())
	}

	// verify original bytes match final bytes!
	require.EqualValues(t, ltd.OrigBytes[:readLen], finalBytes)
}
