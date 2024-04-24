package wallet

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/kmswallet_ipfsunion"
)

type DefWallet struct {
	keystore types.KeyStore

	Local     *LocalWallet
	KMSWallet *kmswallet.KMSWallet

	lk sync.Mutex
}

func NewDefWallet(keystore types.KeyStore, local *LocalWallet) *DefWallet {
	defW := new(DefWallet)
	defW.keystore = keystore
	defW.Local = local

	return defW
}

func (w *DefWallet) SetKMSWallet(kmsW *kmswallet.KMSWallet) {
	w.KMSWallet = kmsW
}

func (w *DefWallet) GetDefault() (address.Address, error) {
	w.lk.Lock()
	defer w.lk.Unlock()

	ki, err := w.keystore.Get(KDefault)
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to get default key: %w", err)
	}

	k, err := NewKey(ki)
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to read default key from keystore: %w", err)
	}

	return k.Address, nil
}

func (w *DefWallet) SetDefault(a address.Address) error {
	w.lk.Lock()
	defer w.lk.Unlock()

	var ki types.KeyInfo
	if w.KMSWallet != nil {
		kiP, err := w.KMSWallet.WalletExport(context.Background(), a)
		if err != nil {
			return fmt.Errorf("failed to find the responding key from KMSWallet: %s, err: %v", a.String(), err)
		}

		ki = *kiP
	} else {
		kiV, err := w.Local.getKeyForDef(a)
		if err != nil {
			return err
		}

		ki = kiV
	}

	if err := w.keystore.Delete(KDefault); err != nil {
		if !xerrors.Is(err, types.ErrKeyInfoNotFound) {
			log.Warnf("failed to unregister current default key: %s", err)
		}
	}

	if err := w.keystore.Put(KDefault, ki); err != nil {
		return err
	}

	return nil
}
