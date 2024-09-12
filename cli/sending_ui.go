package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Kubuxu/imtui"
	"github.com/gdamore/tcell/v2"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
)

func InteractiveSend(ctx context.Context, cctx *cli.Context, srv ServicesAPI,
	proto *api.MessagePrototype) (*types.SignedMessage, error) {

	msg, checks, err := srv.PublishMessage(ctx, proto, cctx.Bool("force") || cctx.Bool("force-send"))
	printer := cctx.App.Writer
	if errors.Is(err, ErrCheckFailed) {
		if !cctx.Bool("interactive") {
			_, _ = fmt.Fprintf(printer, "Following checks have failed:\n")
			printChecks(printer, checks, proto.Message.Cid())
		} else {
			proto, err = resolveChecks(ctx, srv, cctx.App.Writer, proto, checks)
			if err != nil {
				return nil, xerrors.Errorf("from UI: %w", err)
			}

			msg, _, err = srv.PublishMessage(ctx, proto, true)
		}
	}
	if err != nil {
		return nil, xerrors.Errorf("publishing message: %w", err)
	}

	return msg, nil
}

var interactiveSolves = map[api.CheckStatusCode]bool{
	api.CheckStatusMessageMinBaseFee:        true,
	api.CheckStatusMessageBaseFee:           true,
	api.CheckStatusMessageBaseFeeLowerBound: true,
	api.CheckStatusMessageBaseFeeUpperBound: true,
}

func baseFeeFromHints(hint map[string]interface{}) big.Int {
	bHint, ok := hint["baseFee"]
	if !ok {
		return big.Zero()
	}
	bHintS, ok := bHint.(string)
	if !ok {
		return big.Zero()
	}

	var err error
	baseFee, err := big.FromString(bHintS)
	if err != nil {
		return big.Zero()
	}
	return baseFee
}

func resolveChecks(ctx context.Context, s ServicesAPI, printer io.Writer,
	proto *api.MessagePrototype, checkGroups [][]api.MessageCheckStatus,
) (*api.MessagePrototype, error) {

	_, _ = fmt.Fprintf(printer, "Following checks have failed:\n")
	printChecks(printer, checkGroups, proto.Message.Cid())

	if feeCapBad, baseFee := isFeeCapProblem(checkGroups, proto.Message.Cid()); feeCapBad {
		_, _ = fmt.Fprintf(printer, "Fee of the message can be adjusted\n")
		if askUser(printer, "Do you wish to do that? [Yes/no]: ", true) {
			var err error
			proto, err = runFeeCapAdjustmentUI(proto, baseFee)
			if err != nil {
				return nil, err
			}
		}
		checks, err := s.RunChecksForPrototype(ctx, proto)
		if err != nil {
			return nil, err
		}
		_, _ = fmt.Fprintf(printer, "Following checks still failed:\n")
		printChecks(printer, checks, proto.Message.Cid())
	}

	if !askUser(printer, "Do you wish to send this message? [yes/No]: ", false) {
		return nil, ErrAbortedByUser
	}
	return proto, nil
}

var ErrAbortedByUser = errors.New("aborted by user")

func printChecks(printer io.Writer, checkGroups [][]api.MessageCheckStatus, protoCid cid.Cid) {
	for _, checks := range checkGroups {
		for _, c := range checks {
			if c.OK {
				continue
			}
			aboutProto := c.Cid.Equals(protoCid)
			msgName := "current"
			if !aboutProto {
				msgName = c.Cid.String()
			}
			_, _ = fmt.Fprintf(printer, "%s message failed a check %s: %s\n", msgName, c.Code, c.Err)
		}
	}
}

func askUser(printer io.Writer, q string, def bool) bool {
	var resp string
	_, _ = fmt.Fprint(printer, q)
	_, _ = fmt.Scanln(&resp)
	resp = strings.ToLower(resp)
	if len(resp) == 0 {
		return def
	}
	return resp[0] == 'y'
}

func isFeeCapProblem(checkGroups [][]api.MessageCheckStatus, protoCid cid.Cid) (bool, big.Int) {
	baseFee := big.Zero()
	yes := false
	for _, checks := range checkGroups {
		for _, c := range checks {
			if c.OK {
				continue
			}
			aboutProto := c.Cid.Equals(protoCid)
			if aboutProto && interactiveSolves[c.Code] {
				yes = true
				if baseFee.IsZero() {
					baseFee = baseFeeFromHints(c.Hint)
				}
			}
		}
	}
	if baseFee.IsZero() {
		// this will only be the case if failing check is: MessageMinBaseFee
		baseFee = big.NewInt(buildconstants.MinimumBaseFee)
	}

	return yes, baseFee
}

func runFeeCapAdjustmentUI(proto *api.MessagePrototype, baseFee abi.TokenAmount) (*api.MessagePrototype, error) {
	t, err := imtui.NewTui()
	if err != nil {
		return nil, err
	}

	maxFee := big.Mul(proto.Message.GasFeeCap, big.NewInt(proto.Message.GasLimit))
	send := false
	t.PushScene(feeUI(baseFee, proto.Message.GasLimit, &maxFee, &send))

	err = t.Run()
	if err != nil {
		return nil, err
	}
	if !send {
		return nil, fmt.Errorf("aborted by user")
	}

	proto.Message.GasFeeCap = big.Div(maxFee, big.NewInt(proto.Message.GasLimit))

	return proto, nil
}

func feeUI(baseFee abi.TokenAmount, gasLimit int64, maxFee *abi.TokenAmount, send *bool) func(*imtui.Tui) error {
	orignalMaxFee := *maxFee
	required := big.Mul(baseFee, big.NewInt(gasLimit))
	safe := big.Mul(required, big.NewInt(10))

	price := fmt.Sprintf("%s", types.FIL(*maxFee).Unitless())

	return func(t *imtui.Tui) error {
		if t.CurrentKey != nil {
			if t.CurrentKey.Key() == tcell.KeyRune {
				pF, err := types.ParseFIL(price)
				switch t.CurrentKey.Rune() {
				case 's', 'S':
					price = types.FIL(safe).Unitless()
				case '+':
					if err == nil {
						p := big.Mul(big.Int(pF), types.NewInt(11))
						p = big.Div(p, types.NewInt(10))
						price = fmt.Sprintf("%s", types.FIL(p).Unitless())
					}
				case '-':
					if err == nil {
						p := big.Mul(big.Int(pF), types.NewInt(10))
						p = big.Div(p, types.NewInt(11))
						price = fmt.Sprintf("%s", types.FIL(p).Unitless())
					}
				default:
				}
			}

			if t.CurrentKey.Key() == tcell.KeyEnter {
				*send = true
				t.PopScene()
				return nil
			}
		}

		defS := tcell.StyleDefault

		row := 0
		t.Label(0, row, "Fee of the message is too low.", defS)
		row++

		t.Label(0, row, fmt.Sprintf("Your configured maximum fee is: %s FIL",
			types.FIL(orignalMaxFee).Unitless()), defS)
		row++
		t.Label(0, row, fmt.Sprintf("Required maximum fee for the message: %s FIL",
			types.FIL(required).Unitless()), defS)
		row++
		w := t.Label(0, row, fmt.Sprintf("Safe maximum fee for the message: %s FIL",
			types.FIL(safe).Unitless()), defS)
		t.Label(w, row, "   Press S to use it", defS)
		row++

		w = t.Label(0, row, "Current Maximum Fee: ", defS)

		w += t.EditFieldFiltered(w, row, 14, &price, imtui.FilterDecimal, defS.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))

		w += t.Label(w, row, " FIL", defS)

		pF, err := types.ParseFIL(price)
		*maxFee = abi.TokenAmount(pF)
		if err != nil {
			w += t.Label(w, row, " invalid price", defS.Foreground(tcell.ColorMaroon).Bold(true))
		} else if maxFee.GreaterThanEqual(safe) {
			w += t.Label(w, row, " SAFE", defS.Foreground(tcell.ColorDarkGreen).Bold(true))
		} else if maxFee.GreaterThanEqual(required) {
			w += t.Label(w, row, " low", defS.Foreground(tcell.ColorYellow).Bold(true))
			over := big.Div(big.Mul(*maxFee, big.NewInt(100)), required)
			w += t.Label(w, row,
				fmt.Sprintf(" %.1fx over the minimum", float64(over.Int64())/100.0), defS)
		} else {
			w += t.Label(w, row, " too low", defS.Foreground(tcell.ColorRed).Bold(true))
		}
		row += 2

		t.Label(0, row, fmt.Sprintf("Current Base Fee is: %s", types.FIL(baseFee).Nano()), defS)
		row++
		t.Label(0, row, fmt.Sprintf("Resulting FeeCap is: %s",
			types.FIL(big.Div(*maxFee, big.NewInt(gasLimit))).Nano()), defS)
		row++
		t.Label(0, row, "You can use '+' and '-' to adjust the fee.", defS)

		return nil
	}
}
