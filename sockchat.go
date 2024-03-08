package main

import (
	"log"

	"y-a-t-s/sockchat/config"
	"y-a-t-s/sockchat/socket"
	"y-a-t-s/sockchat/tui"
)

func main() {
	cfg, err := config.ParseArgs()
	if err != nil {
		log.Fatal(err)
	}

	sock, err := socket.NewSocket(cfg)
	if err != nil {
		log.Fatal(err)
	}
	sock.Start()
	tui.InitUI(sock, cfg).App.Run()
}
