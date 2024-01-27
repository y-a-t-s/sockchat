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

	ssn := socket.NewSession()
	tui.InitUI(ssn).App.Run()
}
