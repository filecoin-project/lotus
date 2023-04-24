package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/filecoin-project/go-statemachine/fsm"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
)

func storageDealStatusCmp(a, b fsm.StateKey) bool {
	aDealStatus := a.(storagemarket.StorageDealStatus)
	bDealStatus := b.(storagemarket.StorageDealStatus)
	return aDealStatus < bDealStatus
}

func retrievalDealStatusCmp(a, b fsm.StateKey) bool {
	aDealStatus := a.(retrievalmarket.DealStatus)
	bDealStatus := b.(retrievalmarket.DealStatus)
	return aDealStatus < bDealStatus
}

func updateOnChanged(name string, writeContents func(w io.Writer) error) error {
	input, err := os.Open(name)
	if err != nil {
		return err
	}
	orig, err := ioutil.ReadAll(input)
	if err != nil {
		return err
	}
	err = input.Close()
	if err != nil {
		return err
	}
	buf := new(bytes.Buffer)
	err = writeContents(buf)
	if err != nil {
		return err
	}
	if !bytes.Equal(orig, buf.Bytes()) {
		file, err := os.Create(name)
		if err != nil {
			return err
		}
		_, err = file.Write(buf.Bytes())
		if err != nil {
			return err
		}
		err = file.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func main() {

	err := updateOnChanged("./docs/storageclient.mmd", func(w io.Writer) error {
		return fsm.GenerateUML(w, fsm.MermaidUML, storageimpl.ClientFSMParameterSpec, storagemarket.DealStates, storagemarket.ClientEvents, []fsm.StateKey{storagemarket.StorageDealUnknown}, false, storageDealStatusCmp)
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = updateOnChanged("./docs/storageprovider.mmd", func(w io.Writer) error {
		return fsm.GenerateUML(w, fsm.MermaidUML, storageimpl.ProviderFSMParameterSpec, storagemarket.DealStates, storagemarket.ProviderEvents, []fsm.StateKey{storagemarket.StorageDealUnknown}, false, storageDealStatusCmp)
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = updateOnChanged("./docs/retrievalclient.mmd", func(w io.Writer) error {
		return fsm.GenerateUML(w, fsm.MermaidUML, retrievalimpl.ClientFSMParameterSpec, retrievalmarket.DealStatuses, retrievalmarket.ClientEvents, []fsm.StateKey{retrievalmarket.DealStatusNew}, false, retrievalDealStatusCmp)
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = updateOnChanged("./docs/retrievalprovider.mmd", func(w io.Writer) error {
		return fsm.GenerateUML(w, fsm.MermaidUML, retrievalimpl.ProviderFSMParameterSpec, retrievalmarket.DealStatuses, retrievalmarket.ProviderEvents, []fsm.StateKey{retrievalmarket.DealStatusNew}, false, retrievalDealStatusCmp)
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
