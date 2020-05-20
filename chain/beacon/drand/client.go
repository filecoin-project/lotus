package drand

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type DrandHttpClient struct {
	Peer string
}

type DrandNode struct {
	Address string
	Key     string
	Tls     bool
}

type GroupInfo struct {
	Nodes       []DrandNode
	Threshold   int
	Period      int
	GenesisTime int      `json:"genesis_time"`
	GenesisSeed string   `json:"genesis_seed"`
	DistKey     []string `json:"dist_key"`
}

func (c *DrandHttpClient) get(endpoint string, out interface{}) error {
	resp, err := http.DefaultClient.Get("https://" + c.Peer + endpoint)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return err
	}

	return nil
}

func (c *DrandHttpClient) GroupInfo(ctx context.Context) (*GroupInfo, error) {
	var gi GroupInfo
	if err := c.get("/api/info/group", &gi); err != nil {
		return nil, err
	}

	return &gi, nil
}

type RandEntry struct {
	Round         uint64
	Signature     string
	PrevSignature string `json:"previous_signature"`
}

func (c *DrandHttpClient) GetRound(ctx context.Context, round uint64) (*RandEntry, error) {
	var rand RandEntry

	if err := c.get(fmt.Sprintf("/api/public/%d", round), &rand); err != nil {
		return nil, err
	}

	return &rand, nil
}
