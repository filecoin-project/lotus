package main

import (
	"context"
	"log"
	"os"
	"time"

	client "github.com/moran666666/sector-counter/client"
	server "github.com/moran666666/sector-counter/server"
)

func main() {
	os.Setenv("SC_LISTEN", "127.0.0.1:1357")
	// os.Setenv("SECTOR_COUNTER", "0")
	go server.Run()
	time.Sleep(time.Second * 6)
	for i := 0; i < 6; i++ {
		sid, err := client.NewClient().GetSectorID(context.Background(), "")
		if err != nil {
			return
		}
		log.Println("curn sector id: ", sid)
	}
}
