package storiface

import "errors"

var ErrSectorNotFound = errors.New("sector not found")

type UnpaddedByteIndex uint64
