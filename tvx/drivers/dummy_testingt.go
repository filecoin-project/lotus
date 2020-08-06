package drivers

import (
	"fmt"
	"os"
)

var T dummyTestingT

type dummyTestingT struct{}

func (d dummyTestingT) FailNow() {
	panic("fail now")
}

func (d dummyTestingT) Errorf(format string, args ...interface{}) {
	fmt.Printf(format, args)
	os.Exit(1)
}

func (d dummyTestingT) Fatalf(format string, args ...interface{}) {
	d.Errorf(format, args)
}

func (d dummyTestingT) Fatal(args ...interface{}) {
	fmt.Println(args)
	os.Exit(1)
}
