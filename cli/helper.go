package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/filecoin-project/lotus/build"
	ufcli "github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

type PrintHelpErr struct {
	Err error
	Ctx *ufcli.Context
}

func (e *PrintHelpErr) Error() string {
	return e.Err.Error()
}

func (e *PrintHelpErr) Unwrap() error {
	return e.Err
}

func (e *PrintHelpErr) Is(o error) bool {
	_, ok := o.(*PrintHelpErr)
	return ok
}

func ShowHelp(cctx *ufcli.Context, err error) error {
	return &PrintHelpErr{Err: err, Ctx: cctx}
}

func RunApp(app *ufcli.App) {
	defer func() {
		if r := recover(); r != nil {
			// Generate report in LOTUS_PATH and re-raise panic
			build.GeneratePanicReport(os.Getenv("LOTUS_PATH"), "app_panic")
			panic(r)
		}
	}()
	if err := app.Run(os.Args); err != nil {
		if os.Getenv("LOTUS_DEV") != "" {
			log.Warnf("%+v", err)
		} else {
			fmt.Fprintf(os.Stderr, "ERROR: %s\n\n", err) // nolint:errcheck
		}
		var phe *PrintHelpErr
		if xerrors.As(err, &phe) {
			_ = ufcli.ShowCommandHelp(phe.Ctx, phe.Ctx.Command.Name)
		}
		os.Exit(1)
	}
}

type AppFmt struct {
	app   *ufcli.App
	Stdin io.Reader
}

func NewAppFmt(a *ufcli.App) *AppFmt {
	var stdin io.Reader
	istdin, ok := a.Metadata["stdin"]
	if ok {
		stdin = istdin.(io.Reader)
	} else {
		stdin = os.Stdin
	}
	return &AppFmt{app: a, Stdin: stdin}
}

func (a *AppFmt) Print(args ...interface{}) {
	fmt.Fprint(a.app.Writer, args...)
}

func (a *AppFmt) Println(args ...interface{}) {
	fmt.Fprintln(a.app.Writer, args...)
}

func (a *AppFmt) Printf(fmtstr string, args ...interface{}) {
	fmt.Fprintf(a.app.Writer, fmtstr, args...)
}

func (a *AppFmt) Scan(args ...interface{}) (int, error) {
	return fmt.Fscan(a.Stdin, args...)
}
