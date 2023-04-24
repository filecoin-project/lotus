package stores

import "golang.org/x/xerrors"

var ErrNotFound = xerrors.New("not found")

func IsNotFound(err error) bool {
	return xerrors.Is(err, ErrNotFound)
}
