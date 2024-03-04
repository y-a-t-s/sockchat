package main

import (
	"log"

	"y-a-t-s/sockchat/socket"
	"y-a-t-s/sockchat/tui"
)

func main() {
	if err := loadEnv(); err != nil {
		log.Fatal("Could not process .env\n", err)
	}

	sock, err := socket.NewSocket()
	if err != nil {
		log.Fatal(err)
	}
	err = sock.Start()
	if err != nil {
		log.Fatal(err)
	}
	tui.InitUI(sock).App.Run()
}
