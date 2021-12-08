package dagstore

import (
	"context"
	"io"
	"io/ioutil"
	"net/url"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	blocksutil "github.com/ipfs/go-ipfs-blocksutil"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/dagstore/mount"
	mock_dagstore "github.com/filecoin-project/lotus/markets/dagstore/mocks"
)

func TestLotusMount(t *testing.T) {
	ctx := context.Background()
	bgen := blocksutil.NewBlockGenerator()
	cid := bgen.Next().Cid()

	mockCtrl := gomock.NewController(t)
	// when test is done, assert expectations on all mock objects.
	defer mockCtrl.Finish()

	// create a mock lotus api that returns the reader we want
	mockLotusMountAPI := mock_dagstore.NewMockMinerAPI(mockCtrl)

	mockLotusMountAPI.EXPECT().IsUnsealed(gomock.Any(), cid).Return(true, nil).Times(1)

	mr1 := struct {
		io.ReadCloser
		io.ReaderAt
		io.Seeker
	}{
		ReadCloser: ioutil.NopCloser(strings.NewReader("testing")),
		ReaderAt:   nil,
		Seeker:     nil,
	}
	mr2 := struct {
		io.ReadCloser
		io.ReaderAt
		io.Seeker
	}{
		ReadCloser: ioutil.NopCloser(strings.NewReader("testing")),
		ReaderAt:   nil,
		Seeker:     nil,
	}

	mockLotusMountAPI.EXPECT().FetchUnsealedPiece(gomock.Any(), cid).Return(mr1, nil).Times(1)
	mockLotusMountAPI.EXPECT().FetchUnsealedPiece(gomock.Any(), cid).Return(mr2, nil).Times(1)
	mockLotusMountAPI.EXPECT().GetUnpaddedCARSize(ctx, cid).Return(uint64(100), nil).Times(1)

	mnt, err := NewLotusMount(cid, mockLotusMountAPI)
	require.NoError(t, err)
	info := mnt.Info()
	require.Equal(t, info.Kind, mount.KindRemote)

	// fetch and assert success
	rd, err := mnt.Fetch(context.Background())
	require.NoError(t, err)

	bz, err := ioutil.ReadAll(rd)
	require.NoError(t, err)
	require.NoError(t, rd.Close())
	require.Equal(t, []byte("testing"), bz)

	stat, err := mnt.Stat(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 100, stat.Size)

	// serialize url then deserialize from mount template -> should get back
	// the same mount
	url := mnt.Serialize()
	mnt2 := mountTemplate(mockLotusMountAPI)
	err = mnt2.Deserialize(url)
	require.NoError(t, err)

	// fetching on this mount should get us back the same data.
	rd, err = mnt2.Fetch(context.Background())
	require.NoError(t, err)
	bz, err = ioutil.ReadAll(rd)
	require.NoError(t, err)
	require.NoError(t, rd.Close())
	require.Equal(t, []byte("testing"), bz)
}

func TestLotusMountDeserialize(t *testing.T) {
	api := &minerAPI{}

	bgen := blocksutil.NewBlockGenerator()
	cid := bgen.Next().Cid()

	// success
	us := lotusScheme + "://" + cid.String()
	u, err := url.Parse(us)
	require.NoError(t, err)

	mnt := mountTemplate(api)
	err = mnt.Deserialize(u)
	require.NoError(t, err)

	require.Equal(t, cid, mnt.PieceCid)
	require.Equal(t, api, mnt.API)

	// fails if cid is not valid
	us = lotusScheme + "://" + "rand"
	u, err = url.Parse(us)
	require.NoError(t, err)
	err = mnt.Deserialize(u)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse PieceCid")
}

func TestLotusMountRegistration(t *testing.T) {
	ctx := context.Background()
	bgen := blocksutil.NewBlockGenerator()
	cid := bgen.Next().Cid()

	// success
	us := lotusScheme + "://" + cid.String()
	u, err := url.Parse(us)
	require.NoError(t, err)

	mockCtrl := gomock.NewController(t)
	// when test is done, assert expectations on all mock objects.
	defer mockCtrl.Finish()

	mockLotusMountAPI := mock_dagstore.NewMockMinerAPI(mockCtrl)
	registry := mount.NewRegistry()
	err = registry.Register(lotusScheme, mountTemplate(mockLotusMountAPI))
	require.NoError(t, err)

	mnt, err := registry.Instantiate(u)
	require.NoError(t, err)

	mockLotusMountAPI.EXPECT().IsUnsealed(ctx, cid).Return(true, nil)
	mockLotusMountAPI.EXPECT().GetUnpaddedCARSize(ctx, cid).Return(uint64(100), nil).Times(1)
	stat, err := mnt.Stat(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 100, stat.Size)
	require.True(t, stat.Ready)
}
