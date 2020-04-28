package main

import (
	"encoding/json"
	"io/ioutil"
	"time"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"golang.org/x/xerrors"
)

type JDuration time.Duration

func (d *JDuration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = JDuration(dur)
	return nil
}

func (d JDuration) MarshalJSON() ([]byte, error) {
	return []byte("\"" + time.Duration(d).String() + "\""), nil
}
func (d JDuration) D() time.Duration {
	return time.Duration(d)
}

type Params struct {
	Priv        ffi.PrivateKey
	GenesisTime time.Time
	Round       JDuration
}

func createNewParams(fname string) error {
	defParams := Params{
		GenesisTime: time.Now().UTC().Round(1 * time.Second),
		Round:       JDuration(30 * time.Second),
	}
	pk := ffi.PrivateKeyGenerate()
	defParams.Priv = pk
	params, err := json.Marshal(defParams)
	if err != nil {
		return xerrors.Errorf("marshaling params: %w", err)
	}

	err = ioutil.WriteFile(fname, params, 0600)
	if err != nil {
		return xerrors.Errorf("writing file: %w", err)
	}
	return nil
}

func loadParams(fname string) (*Params, error) {
	b, err := ioutil.ReadFile(fname)
	if err != nil {
		return nil, xerrors.Errorf("reading file: %w", err)
	}
	var p Params
	err = json.Unmarshal(b, &p)
	if err != nil {
		return nil, xerrors.Errorf("unmarshal: %w", err)
	}

	return &p, nil
}
