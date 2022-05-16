package kit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
)

type ItestdNotif struct {
	NodeType string // api env var name
	TestName string
	Api      string
}

func sendItestdNotif(nodeType, testName, apiAddr string) {
	td := os.Getenv("LOTUS_ITESTD")
	if td == "" {
		// not running
		return
	}

	notif := ItestdNotif{
		NodeType: nodeType,
		TestName: testName,
		Api:      apiAddr,
	}
	nb, err := json.Marshal(&notif)
	if err != nil {
		return
	}

	if _, err := http.Post(td, "application/json", bytes.NewReader(nb)); err != nil { // nolint:gosec
		return
	}
}
