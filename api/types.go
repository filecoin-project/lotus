package api

import (
	"encoding/json"

	ma "github.com/multiformats/go-multiaddr"
)

type DealState = uint64

const (
	DealUnknown  = DealState(iota)
	DealRejected // Provider didn't like the proposal
	DealAccepted // Proposal accepted, data moved
	DealStaged   // Data put into the sector
	DealSealing  // Data in process of being sealed

	DealFailed
	DealComplete

	// Internal

	DealError // deal failed with an unexpected error

	DealNoUpdate = DealUnknown
)

var DealStates = []string{
	"DealUnknown",
	"DealRejected",
	"DealAccepted",
	"DealStaged",
	"DealSealing",
	"DealFailed",
	"DealComplete",
	"DealError",
}

// TODO: check if this exists anywhere else
type MultiaddrSlice []ma.Multiaddr

func (m *MultiaddrSlice) UnmarshalJSON(raw []byte) (err error) {
	var temp []string
	if err := json.Unmarshal(raw, &temp); err != nil {
		return err
	}

	res := make([]ma.Multiaddr, len(temp))
	for i, str := range temp {
		res[i], err = ma.NewMultiaddr(str)
		if err != nil {
			return err
		}
	}
	*m = res
	return nil
}

var _ json.Unmarshaler = new(MultiaddrSlice)
