package main

import (
	"log"
	"os"
	"ws-agent/internal/desktopcapture"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	if err := desktopcapture.Run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
