package deals

import (
	"io"
	"io/ioutil"
	"os"
)

func withTemp(r io.Reader, cb func(string) error) error {
	f, err := ioutil.TempFile(os.TempDir(), "lotus-temp-")
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	err = cb(f.Name())
	if err != nil {
		os.Remove(f.Name())
		return err
	}

	return os.Remove(f.Name())
}
