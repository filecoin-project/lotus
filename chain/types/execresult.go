package types

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"
)

type ExecutionTrace struct {
	Msg        *Message
	MsgRct     *MessageReceipt
	Error      string
	Duration   time.Duration
	GasCharges []*GasTrace

	Subcalls []ExecutionTrace
}

type GasTrace struct {
	Name       string
	Location   string
	TotalGas   int64
	ComputeGas int64
	StorageGas int64

	TimeTaken time.Duration

	Callers []uintptr `json:"-"`
}

func (gt *GasTrace) MarshalJSON() ([]byte, error) {
	type GasTraceCopy GasTrace
	if gt.Location == "" {
		if len(gt.Callers) != 0 {
			frames := runtime.CallersFrames(gt.Callers)
			for {
				frame, more := frames.Next()
				fn := strings.Split(frame.Function, "/")

				split := strings.Split(frame.File, "/")
				file := strings.Join(split[len(split)-2:], "/")
				gt.Location += fmt.Sprintf("%s@%s:%d", fn[len(fn)-1], file, frame.Line)
				if !more {
					break
				}
				gt.Location += "|"
			}
		} else {
			gt.Location = "n/a"
		}
	}

	cpy := (*GasTraceCopy)(gt)
	return json.Marshal(cpy)
}
