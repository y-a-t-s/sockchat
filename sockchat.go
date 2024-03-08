package main

import (
	"log"

	"y-a-t-s/sockchat/config"
	"y-a-t-s/sockchat/socket"
	"y-a-t-s/sockchat/tui"
)

func main() {
	if err := config.LoadEnv(); err != nil {
		log.Fatal("Could not process .env\n", err)
	}

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
