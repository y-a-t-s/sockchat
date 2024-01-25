package main

import (
	"log"

	"y-a-t-s/sockchat/socket"
	"y-a-t-s/sockchat/tui"
)

func main() {
	if err := loadEnv(); err != nil {
		log.Fatal("Could not process .env", err)
	}

	sock := socket.Init()
	tui.InitUI(sock).App.Run()
}
