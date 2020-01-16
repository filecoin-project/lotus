package main

import (
	"testing"

	cid "github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"
)

func TestAppendCIDsToWindow(t *testing.T) {
	assert := assert.New(t)
	var window CidWindow
	threshold := 3
	cid0 := makeCID("0")
	cid1 := makeCID("1")
	cid2 := makeCID("2")
	cid3 := makeCID("3")
	window = appendCIDsToWindow(window, []cid.Cid{cid0}, threshold)
	window = appendCIDsToWindow(window, []cid.Cid{cid1}, threshold)
	window = appendCIDsToWindow(window, []cid.Cid{cid2}, threshold)
	window = appendCIDsToWindow(window, []cid.Cid{cid3}, threshold)
	assert.Len(window, 3)
	assert.Equal(window[0][0], cid1)
	assert.Equal(window[1][0], cid2)
	assert.Equal(window[2][0], cid3)
}

func TestCheckWindow(t *testing.T) {
	assert := assert.New(t)
	ch := make(chan CidWindow, 1)
	och := make(chan bool, 1)
	threshold := 3
	go func() {
		och <- checkWindow(ch, threshold)
	}()
	var healthyHeadCheckWindow CidWindow
	healthyHeadCheckWindow = appendCIDsToWindow(healthyHeadCheckWindow, []cid.Cid{
		makeCID("abcd"),
	}, threshold)
	healthyHeadCheckWindow = appendCIDsToWindow(healthyHeadCheckWindow, []cid.Cid{
		makeCID("bbcd"),
		makeCID("bbfe"),
	}, threshold)
	healthyHeadCheckWindow = appendCIDsToWindow(healthyHeadCheckWindow, []cid.Cid{
		makeCID("bbcd"),
		makeCID("bbfe"),
	}, threshold)
	ch <- healthyHeadCheckWindow
	select {
	case ok := <-och:
		assert.True(ok)
	}
	go func() {
		och <- checkWindow(ch, threshold)
	}()
	var unhealthyHeadCheckWindow CidWindow
	unhealthyHeadCheckWindow = appendCIDsToWindow(unhealthyHeadCheckWindow, []cid.Cid{
		makeCID("abcd"),
		makeCID("fbcd"),
	}, threshold)
	unhealthyHeadCheckWindow = appendCIDsToWindow(unhealthyHeadCheckWindow, []cid.Cid{
		makeCID("abcd"),
		makeCID("fbcd"),
	}, threshold)
	unhealthyHeadCheckWindow = appendCIDsToWindow(unhealthyHeadCheckWindow, []cid.Cid{
		makeCID("abcd"),
		makeCID("fbcd"),
	}, threshold)
	ch <- unhealthyHeadCheckWindow
	select {
	case ok := <-och:
		assert.False(ok)
	}

}

func makeCID(s string) cid.Cid {
	h1, err := mh.Sum([]byte(s), mh.SHA2_256, -1)
	if err != nil {
		log.Fatal(err)
	}
	return cid.NewCidV1(0x55, h1)
}
