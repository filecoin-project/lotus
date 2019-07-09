package repo

import (
	"testing"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
)

func TestMemRepo(t *testing.T) {
	repo := NewMemory(nil)
	apima, err := repo.APIEndpoint()
	if assert.Error(t, err) {
		assert.Equal(t, ErrNoAPIEndpoint, err)
	}
	assert.Nil(t, apima, "with no api endpoint, return should be nil")

	lrepo, err := repo.Lock()
	assert.NoError(t, err, "should be able to lock once")
	assert.NotNil(t, lrepo, "locked repo shouldn't be nil")

	{
		lrepo2, err := repo.Lock()
		if assert.Error(t, err) {
			assert.Equal(t, ErrRepoAlreadyLocked, err)
		}
		assert.Nil(t, lrepo2, "with locked repo errors, nil should be returned")
	}

	err = lrepo.Close()
	assert.NoError(t, err, "should be able to unlock")

	lrepo, err = repo.Lock()
	assert.NoError(t, err, "should be able to relock")
	assert.NotNil(t, lrepo, "locked repo shouldn't be nil")

	ma, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/43244")
	assert.NoError(t, err, "creating multiaddr shouldn't error")

	err = lrepo.SetAPIEndpoint(ma)
	assert.NoError(t, err, "setting multiaddr shouldn't error")

	apima, err = repo.APIEndpoint()
	assert.NoError(t, err, "setting multiaddr shouldn't error")
	assert.Equal(t, ma, apima, "returned API multiaddr should be the same")

	err = lrepo.Close()
	assert.NoError(t, err, "should be able to close")

	apima, err = repo.APIEndpoint()

	if assert.Error(t, err) {
		assert.Equal(t, ErrNoAPIEndpoint, err, "after closing repo, api should be nil")
	}
	assert.Nil(t, apima, "with closed repo, apima should be set back to nil")

}
