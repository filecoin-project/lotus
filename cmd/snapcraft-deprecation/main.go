package main

import "fmt"

const notice = "Lotus is not distributed through Snap anymore (https://github.com/filecoin-project/lotus/issues/10002)\n" +
	"Please install Lotus using another method from https://lotus.filecoin.io/lotus/install/prerequisites/"

func main() {
	fmt.Println(notice)
}
