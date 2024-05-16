package main

import (
	"context"
	"encoding/gob"
	"fmt"
	"os"
	"reflect"

	"github.com/samber/lo"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/lib/ffiselect"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var ffiCmd = &cli.Command{
	Name:   "ffi",
	Hidden: true,
	Flags: []cli.Flag{
		layersFlag,
	},
	Action: func(cctx *cli.Context) (err error) {
		output := os.NewFile(uintptr(3), "out")

		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic: %v", r)
			}
			if err != nil {
				gob.NewEncoder(output).Encode(ffiselect.ValErr{Val: nil, Err: err})
			}
		}()
		var callInfo ffiselect.FFICall
		if err := gob.NewDecoder(os.Stdin).Decode(&callInfo); err != nil {
			return err
		}

		// wasteful, but *should* work
		depnd, err := deps.GetDeps(cctx.Context, cctx)

		w, err := ffiwrapper.New(&sectorProvider{index: depnd.Si, stor: depnd.LocalStore})
		if err != nil {
			return err
		}

		args := lo.Map(callInfo.Args, func(arg any, i int) reflect.Value {
			return reflect.ValueOf(arg)
		})

		// All methods 1st arg is a context, not passed in.
		args = append([]reflect.Value{reflect.ValueOf(cctx.Context)}, args...)

		resAry := reflect.ValueOf(w).MethodByName(callInfo.Fn).Call(args)
		res := lo.Map(resAry, func(res reflect.Value, i int) any {
			return res.Interface()
		})

		return gob.NewEncoder(output).Encode(ffiselect.ValErr{Val: res, Err: nil})
	},
}

type sectorProvider struct {
	index paths.SectorIndex
	stor  *paths.Local
}

func (l *sectorProvider) AcquireSector(ctx context.Context, id storiface.SectorRef, existing storiface.SectorFileType, allocate storiface.SectorFileType, sealing storiface.PathType) (storiface.SectorPaths, func(), error) {
	if allocate != storiface.FTNone {
		return storiface.SectorPaths{}, nil, xerrors.New("read-only storage")
	}

	ctx, cancel := context.WithCancel(ctx)

	// use TryLock to avoid blocking
	locked, err := l.index.StorageTryLock(ctx, id.ID, existing, storiface.FTNone)
	if err != nil {
		cancel()
		return storiface.SectorPaths{}, nil, xerrors.Errorf("acquiring sector lock: %w", err)
	}
	if !locked {
		cancel()
		return storiface.SectorPaths{}, nil, xerrors.Errorf("failed to acquire sector lock")
	}

	p, _, err := l.stor.AcquireSector(ctx, id, existing, allocate, sealing, storiface.AcquireMove)

	return p, cancel, err
}

func (l *sectorProvider) AcquireSectorCopy(ctx context.Context, id storiface.SectorRef, existing storiface.SectorFileType, allocate storiface.SectorFileType, ptype storiface.PathType) (storiface.SectorPaths, func(), error) {
	if allocate != storiface.FTNone {
		return storiface.SectorPaths{}, nil, xerrors.New("read-only storage")
	}

	ctx, cancel := context.WithCancel(ctx)

	// use TryLock to avoid blocking
	locked, err := l.index.StorageTryLock(ctx, id.ID, existing, storiface.FTNone)
	if err != nil {
		cancel()
		return storiface.SectorPaths{}, nil, xerrors.Errorf("acquiring sector lock: %w", err)
	}
	if !locked {
		cancel()
		return storiface.SectorPaths{}, nil, xerrors.Errorf("failed to acquire sector lock")
	}

	p, _, err := l.stor.AcquireSector(ctx, id, existing, allocate, ptype, storiface.AcquireCopy)

	return p, cancel, err
}
