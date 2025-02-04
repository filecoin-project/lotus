package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	logging "github.com/ipfs/go-log/v2"
	ufcli "github.com/urfave/cli/v2"
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

func IncorrectNumArgs(cctx *ufcli.Context) error {
	return ShowHelp(cctx, fmt.Errorf("incorrect number of arguments, got %d", cctx.NArg()))
}

func RunApp(app *ufcli.App) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-c
		os.Exit(1)
	}()

	if err := app.Run(os.Args); err != nil {
		if cfg := logging.GetConfig(); !(cfg.Stdout || cfg.Stderr) {
			// To avoid printing the error twice while making sure that log file contains the
			// error, check the config and only print it if the output isn't stderr or
			// stdout.
			log.Errorw("Failed to start application", "err", err)
		}
		_, _ = fmt.Fprintf(os.Stderr, "ERROR: %s\n\n", err)
		var phe *PrintHelpErr
		if errors.As(err, &phe) {
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
	_, _ = fmt.Fprint(a.app.Writer, args...)
}

func (a *AppFmt) Println(args ...interface{}) {
	_, _ = fmt.Fprintln(a.app.Writer, args...)
}

func (a *AppFmt) Printf(fmtstr string, args ...interface{}) {
	_, _ = fmt.Fprintf(a.app.Writer, fmtstr, args...)
}

func (a *AppFmt) Scan(args ...interface{}) (int, error) {
	return fmt.Fscan(a.Stdin, args...)
}
